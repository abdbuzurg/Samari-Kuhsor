import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function POST(req: NextRequest) {
  return proxy(req, '/alerts/read', { body: await readBody(req) });
}
