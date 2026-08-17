import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';
import type { Item, ItemListRow } from '@samari/types';

/** GET /api/items — the Товары list. Filters, search, sort and paging are Go's. */
export async function GET(req: NextRequest) {
  return proxy<ItemListRow[]>(req, '/items');
}

/** POST /api/items — create. Requires items:manage, enforced in Go. */
export async function POST(req: NextRequest) {
  return proxy<Item>(req, '/items', { body: await readBody(req) });
}
