import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/analytics — the two website-statistics panels.
 *
 * Guarded on analytics:read in Go, which only admin and director hold. Reads
 * the daily rollup rather than the raw event table, so the panels keep working
 * after the 90-day window has emptied it (docs/01-DECISIONS.md D12).
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/analytics/report');
}
