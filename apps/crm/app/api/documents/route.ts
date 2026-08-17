import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** Документы — docs/05-MODULES.md §14. */
export async function GET(req: NextRequest) {
  return proxy(req, '/documents');
}

export async function POST(req: NextRequest) {
  return proxy(req, '/documents', { body: await readBody(req) });
}
