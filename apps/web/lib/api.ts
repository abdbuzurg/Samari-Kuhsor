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

/**
 * The public site has no sessions.
 *
 * Nobody logs in here, so `authenticated` is always false and no cookie is ever
 * read. The constant is kept so this file stays a copy of the CRM's rather than
 * a fork of it — the two must not drift on how they call Go.
 */
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
  /**
   * Seconds to cache this response for.
   *
   * The CRM never sets it: every screen there is live data behind a login, and a
   * stale stock figure is worse than a slow one. The public site is the opposite
   * case — the catalogue is five products that change when the client edits
   * them, and fetching it fresh for every visitor puts the Go API in the request
   * path of every page view for no benefit.
   *
   * Omitted means no-store, so the default stays the safe one.
   */
  revalidate?: number;
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

  // Only ever true if a caller asks explicitly. Every endpoint the public site
  // uses is under /public/, which Go serves without a session.
  if (opts.authenticated === true) {
    const token = (await cookies()).get(SESSION_COOKIE)?.value;
    if (token) headers['Authorization'] = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${BACKEND_URL}/api/v1${path}`, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      ...(opts.revalidate === undefined
        ? { cache: 'no-store' as const }
        : { next: { revalidate: opts.revalidate } }),
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
  // 204 carries no body, and `Response.json` REFUSES to build one that does —
  // "Invalid response status code 204" from the constructor, surfacing as a 500
  // from a route that had already succeeded. callApi has always understood 204;
  // relay did not, and nothing exercised it until the analytics beacon became
  // the first endpoint to answer with one.
  if (result.status === 204) {
    return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } });
  }

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
