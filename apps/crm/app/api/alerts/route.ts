import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/alerts — the bell and the sidebar count pills, from one call.
 *
 * Serving both from one endpoint is deliberate: two endpoints could disagree
 * about how many open items a module has, and the user would see a badge that
 * does not match the list behind it.
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/alerts');
}
