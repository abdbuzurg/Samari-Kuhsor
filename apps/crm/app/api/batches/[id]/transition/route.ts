import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — moves a batch's status. Which moves are legal, and which need
 *  quality:approve, is decided entirely in Go. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/batches/${id}/transition`, { body: await readBody(req) });
}
