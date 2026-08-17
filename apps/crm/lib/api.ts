import 'server-only';

import { cookies } from 'next/headers';

/**
 * Server-side client for the Go API.
 *
 * `import 'server-only'` at the top is the enforcement, not a convention: if any
 * client component ever imports this module, the build FAILS rather than
 * shipping BACKEND_URL and SERVICE_KEY to the browser.
 *
 * CLAUDE.md §3: "The browser never calls the Go API directly. No backend URL,
 * token or service credential may appear in client-side code."
 *
 * This module is used only inside app/api/* route handlers — the BFF. It
 * proxies and shapes; it makes no authorization decisions, because those live in
 * Go middleware (docs/04-RBAC.md:119).
 */

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://127.0.0.1:8080';
const SERVICE_KEY = process.env.SERVICE_KEY ?? '';

/** The httpOnly cookie the browser holds. Its value is the opaque session token. */
export const SESSION_COOKIE = 'samari_session';

export type ApiResult<T> =
  | { ok: true; status: number; data: T; meta?: unknown }
  | { ok: false; status: number; error: ApiErrorBody };

export interface ApiErrorBody {
  code: string;
  message: string;
  details?: { field: string; code: string; message: string }[];
}

interface CallOptions {
  method?: string;
  body?: unknown;
  /** Send the caller's session token. Off for login, which has none yet. */
  authenticated?: boolean;
  /** Forwarded so the audit trail records the real client, not the BFF. */
  clientIp?: string | null;
  signal?: AbortSignal;
}

/**
 * Calls the Go API.
 *
 * The service key proves the caller is a BFF; it is never an identity. The user
 * is identified only by the Bearer session token, which Go resolves itself
 * (docs/07-IMPLEMENTATION-PLAN.md I8). This function must never send a user id.
 */
export async function callApi<T>(path: string, opts: CallOptions = {}): Promise<ApiResult<T>> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  if (SERVICE_KEY) headers['X-Service-Key'] = SERVICE_KEY;
  if (opts.clientIp) headers['X-Forwarded-For'] = opts.clientIp;

  if (opts.authenticated !== false) {
    const token = (await cookies()).get(SESSION_COOKIE)?.value;
    if (token) headers['Authorization'] = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${BACKEND_URL}/api/v1${path}`, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      cache: 'no-store',
      signal: opts.signal,
    });
  } catch (cause) {
    // The API being unreachable is an infrastructure fault. Report it in the
    // contract's own error shape so the client has exactly one thing to parse,
    // and never surface the backend address in the message.
    console.error('BFF: backend unreachable', cause);
    return {
      ok: false,
      status: 503,
      error: { code: 'internal_error', message: 'Сервис временно недоступен' },
    };
  }

  if (res.status === 204) {
    return { ok: true, status: 204, data: undefined as T };
  }

  const payload = (await res.json().catch(() => null)) as
    | { data?: T; meta?: unknown; error?: ApiErrorBody }
    | null;

  if (!res.ok) {
    return {
      ok: false,
      status: res.status,
      error: payload?.error ?? {
        code: 'internal_error',
        message: 'Внутренняя ошибка сервера',
      },
    };
  }

  // `meta` is carried through, not dropped: a collection response is
  // {data, meta} (docs/03-API-CONTRACT.md §4), and losing meta breaks paging and
  // every total the UI shows.
  return { ok: true, status: res.status, data: payload?.data as T, meta: payload?.meta };
}

/**
 * Relays an API result to the browser, preserving the envelope and status.
 *
 * The BFF deliberately does not reshape errors: the frontend switches on
 * `error.code` (docs/03-API-CONTRACT.md:116), so translating codes here would
 * break every consumer for no gain.
 */
export function relay<T>(result: ApiResult<T>): Response {
  const body = result.ok
    ? result.meta === undefined
      ? { data: result.data }
      : { data: result.data, meta: result.meta }
    : { error: result.error };
  return Response.json(body, {
    status: result.status,
    headers: { 'Cache-Control': 'no-store' },
  });
}

/**
 * Forwards a browser request to the Go API, preserving the query string.
 *
 * This is the shape every module's BFF route reuses. The BFF proxies and shapes;
 * it implements no business rules and makes no authorization decisions, because
 * those live in Go middleware (docs/03-API-CONTRACT.md:19).
 *
 * The query string is forwarded WHOLE rather than allow-listed here. Go already
 * validates every parameter — sort fields against a whitelist, page sizes
 * clamped — and a second, drifting copy of those rules in the BFF is how the two
 * layers start disagreeing about what is valid.
 */
export async function proxy<T>(
  req: Request,
  path: string,
  opts: { method?: string; body?: unknown } = {},
): Promise<Response> {
  const search = new URL(req.url).search;
  return relay(
    await callApi<T>(`${path}${search}`, {
      method: opts.method ?? req.method,
      body: opts.body,
      clientIp: req.headers.get('x-forwarded-for'),
    }),
  );
}

/** Reads a JSON body, tolerating an empty one. */
export async function readBody(req: Request): Promise<unknown> {
  const text = await req.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    // Let Go produce the validation error, so there is one source of error text.
    return text;
  }
}
