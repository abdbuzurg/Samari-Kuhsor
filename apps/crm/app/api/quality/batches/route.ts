import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/quality/batches — Качество и безопасность.
 *
 * Mounted under /quality rather than /batches because the two are different
 * modules with different permissions: /batches belongs to Товары (items:read,
 * for QR issuance), this belongs to Качество (quality:read).
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/quality/batches');
}
