import { NextRequest } from 'next/server';
import { cookies } from 'next/headers';

import { callApi, relay, SESSION_COOKIE, type ApiResult } from '@/lib/api';

interface LoginPayload {
  token: string;
  user: unknown;
}

/**
 * POST /api/auth/login — browser -> BFF -> Go.
 *
 * This is where the session token becomes an httpOnly cookie. The Go API returns
 * the token in its body and never sets a cookie: the cookie lives between the
 * browser and Next.js, and Go only ever sees a Bearer header
 * (docs/07-IMPLEMENTATION-PLAN.md I8).
 *
 * The token is NOT returned to the browser in the response body. httpOnly is the
 * whole point — handing the same value back in JSON would put it within reach of
 * any XSS on the page.
 */
export async function POST(req: NextRequest) {
  const body = await req.json().catch(() => ({}));

  const result: ApiResult<LoginPayload> = await callApi<LoginPayload>('/auth/login', {
    method: 'POST',
    body,
    authenticated: false,
    clientIp: req.headers.get('x-forwarded-for'),
  });

  if (!result.ok) return relay(result);

  const store = await cookies();
  store.set(SESSION_COOKIE, result.data.token, {
    httpOnly: true,
    // Derived from TLS_MODE, never hand-set. Browsers refuse to SEND a Secure
    // cookie over plain HTTP, so during the IP-and-plain-HTTP client-testing
    // phase this must be false or login fails silently — the cookie is set and
    // then never returned (docs/07-IMPLEMENTATION-PLAN.md I24/I25).
    secure: process.env.TLS_MODE !== 'off',
    sameSite: 'lax',
    path: '/',
    maxAge: 60 * 60 * 8, // matches the API's idle timeout
  });

  return Response.json({ data: { user: result.data.user } }, {
    status: 200,
    headers: { 'Cache-Control': 'no-store' },
  });
}
