import { NextRequest } from 'next/server';
import { cookies } from 'next/headers';

import { callApi, SESSION_COOKIE } from '@/lib/api';

/**
 * POST /api/auth/logout.
 *
 * The cookie is cleared even if the API call fails. A user who pressed "выйти"
 * must end up logged out of this browser regardless — leaving a live cookie
 * behind because the backend hiccuped is the worst possible outcome, especially
 * on a shared terminal on the factory floor.
 */
export async function POST(req: NextRequest) {
  const result = await callApi('/auth/logout', {
    method: 'POST',
    clientIp: req.headers.get('x-forwarded-for'),
  });

  const store = await cookies();
  store.delete(SESSION_COOKIE);

  if (!result.ok && result.status >= 500) {
    console.error('BFF: logout failed upstream', result.error);
  }
  return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } });
}
