import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST /api/stock/transfers — two movements in one transaction, so stock is
 *  never in flight and never counted twice. */
export async function POST(req: NextRequest) {
  return proxy(req, '/stock/transfers', { body: await readBody(req) });
}
