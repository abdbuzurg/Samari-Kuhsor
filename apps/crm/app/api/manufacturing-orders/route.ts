import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Производство — docs/05-MODULES.md §6. */
export async function GET(req: NextRequest) {
  return proxy(req, '/manufacturing-orders');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/manufacturing-orders', { body: await readBody(req) });
}
