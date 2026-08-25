import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/**
 * POST /api/analytics — website statistics (docs/01-DECISIONS.md D12).
 *
 * The browser never reaches Go directly (CLAUDE.md §3), so the beacon lands here
 * and is forwarded with the service credential. Go validates every target
 * against the real catalogue, rate-limits on a salted IP hash, and answers 204
 * for anything it does not understand.
 */
export async function POST(req: NextRequest) {
  return proxy(req, '/public/analytics', { body: await readBody(req) });
}
