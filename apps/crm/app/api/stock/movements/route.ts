import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST /api/stock/movements — appends one signed entry to the ledger. The only
 *  way stock changes; there is no endpoint that sets a quantity. */
export async function POST(req: NextRequest) {
  return proxy(req, '/stock/movements', { body: await readBody(req) });
}
