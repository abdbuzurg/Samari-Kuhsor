import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function GET(req: NextRequest) {
  return proxy(req, '/suppliers');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/suppliers', { body: await readBody(req) });
}
