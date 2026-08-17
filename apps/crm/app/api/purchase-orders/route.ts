import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Закупки — docs/05-MODULES.md §8. */
export async function GET(req: NextRequest) {
  return proxy(req, '/purchase-orders');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/purchase-orders', { body: await readBody(req) });
}
