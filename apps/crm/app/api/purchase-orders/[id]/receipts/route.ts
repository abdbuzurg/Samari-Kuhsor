import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — records a goods receipt and posts the matching stock movements. One
 *  transaction in Go: the receipt and the ledger entries are the same event. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/purchase-orders/${id}/receipts`, { body: await readBody(req) });
}
