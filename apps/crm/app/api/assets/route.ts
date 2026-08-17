import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Оборудование и ТО — docs/05-MODULES.md §13. */
export async function GET(req: NextRequest) {
  return proxy(req, '/assets');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/assets', { body: await readBody(req) });
}
