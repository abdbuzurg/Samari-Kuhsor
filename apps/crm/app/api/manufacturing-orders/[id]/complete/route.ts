import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — completion posts the output to quarantine and moves the batch, in one
 *  transaction in Go. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/manufacturing-orders/${id}/complete`, { body: await readBody(req) });
}
