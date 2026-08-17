import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/audit — the audit viewer.
 *
 * Guarded on audit:read in Go, not admin:manage: reading the trail and changing
 * who may do what are different authorities, and an auditor should not need the
 * power to grant themselves anything.
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/audit');
}
