import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';
import type { Item } from '@samari/types';

type Params = { params: Promise<{ id: string }> };

export async function GET(req: NextRequest, { params }: Params) {
  const { id } = await params;
  return proxy<Item>(req, `/items/${encodeURIComponent(id)}`);
}

/** PATCH carries the version it read; a stale one comes back as 409. */
export async function PATCH(req: NextRequest, { params }: Params) {
  const { id } = await params;
  return proxy<Item>(req, `/items/${encodeURIComponent(id)}`, { body: await readBody(req) });
}

/** DELETE tombstones. It carries a version too — deleting a record someone else
 *  just edited must fail the same way editing it would. */
export async function DELETE(req: NextRequest, { params }: Params) {
  const { id } = await params;
  return proxy<void>(req, `/items/${encodeURIComponent(id)}`, { body: await readBody(req) });
}
