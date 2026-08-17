import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Администрирование — docs/05-MODULES.md §18. */
export async function GET(req: NextRequest) {
  return proxy(req, '/admin/roles');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/admin/roles', { body: await readBody(req) });
}
