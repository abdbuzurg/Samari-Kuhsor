import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — appends a shift entry. Append-only: an entry is what someone observed
 *  on the line, and observations are not edited afterwards. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/manufacturing-orders/${id}/entries`, { body: await readBody(req) });
}
