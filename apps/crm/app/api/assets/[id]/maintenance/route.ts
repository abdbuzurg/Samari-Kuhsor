import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/assets/${id}/maintenance`);
}

/** POST — a service record. If the asset was flagged as due, Go returns it to
 *  `running`: leaving that to the operator is how an asset stays amber after it
 *  has been serviced, and the factory learns to ignore the colour. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/assets/${id}/maintenance`, { body: await readBody(req) });
}
