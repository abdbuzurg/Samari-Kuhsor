import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/stock — Склад и запасы.
 *
 * Every quantity in the response is a SUM computed at read time; there is no
 * stored balance to fetch (CLAUDE.md §4.2). The query string is forwarded whole
 * so Go stays the only validator of sort fields and page sizes.
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/stock');
}
