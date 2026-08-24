import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** GET /api/customers · POST /api/customers — Клиенты. Guarded on crm in Go. */
export async function GET(req: NextRequest) {
  return proxy(req, '/customers');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/customers', { body: await readBody(req) });
}
