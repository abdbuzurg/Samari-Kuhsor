import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Продажи — docs/05-MODULES.md §9. */
export async function GET(req: NextRequest) {
  return proxy(req, '/sales-orders');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/sales-orders', { body: await readBody(req) });
}
