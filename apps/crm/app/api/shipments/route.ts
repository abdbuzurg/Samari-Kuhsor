import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Логистика — docs/05-MODULES.md §10. */
export async function GET(req: NextRequest) {
  return proxy(req, '/shipments');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/shipments', { body: await readBody(req) });
}
