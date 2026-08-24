import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/**
 * POST /api/batches — creates a batch.
 *
 * Guarded on items:manage in Go. A batch must be creatable BEFORE the plant
 * produces anything, because QR payloads are printed onto wrappers ordered
 * months in advance (D11). The Go route has existed since T15; nothing in the
 * CRM could reach it.
 */
export async function POST(req: NextRequest) {
  return proxy(req, '/batches', { body: await readBody(req) });
}
