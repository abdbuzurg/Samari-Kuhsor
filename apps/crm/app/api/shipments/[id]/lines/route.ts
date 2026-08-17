import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — loads one batch onto a trip. The released-batch check is in Go: a
 *  lorry leaving Хорог with quarantined product is the failure the whole
 *  quality chain exists to prevent. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/shipments/${id}/lines`, { body: await readBody(req) });
}
