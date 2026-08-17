import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';
import type { Price } from '@samari/types';

/** POST /api/items/{id}/prices — adds a price, superseding the open one. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy<Price>(req, `/items/${encodeURIComponent(id)}/prices`, {
    body: await readBody(req),
  });
}
