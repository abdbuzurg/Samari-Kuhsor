import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — the gate. A draft may quote a batch still in quarantine; confirming
 *  checks every line is released and reserves the stock. Both rules are Go's. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/sales-orders/${id}/confirm`, { body: await readBody(req) });
}
