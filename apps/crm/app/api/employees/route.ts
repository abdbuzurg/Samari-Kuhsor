import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Персонал — docs/05-MODULES.md §12. */
export async function GET(req: NextRequest) {
  return proxy(req, '/employees');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/employees', { body: await readBody(req) });
}
