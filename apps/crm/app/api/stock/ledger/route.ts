import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** GET /api/stock/ledger — the movement history for one position, with a
 *  running balance. This is the answer to "why does it say 480?". */
export async function GET(req: NextRequest) {
  return proxy(req, '/stock/ledger');
}
