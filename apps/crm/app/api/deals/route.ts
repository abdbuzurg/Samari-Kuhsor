import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** GET /api/deals · POST /api/deals — the pipeline. */
export async function GET(req: NextRequest) {
  return proxy(req, '/deals');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/deals', { body: await readBody(req) });
}
