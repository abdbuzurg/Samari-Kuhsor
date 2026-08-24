import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** GET /api/employees/{id} — the Персонал detail view's read. Guarded on
 *  hr:read in Go; there is deliberately no public counterpart (T23). */
export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/employees/${id}`);
}

export async function PATCH(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/employees/${id}`, { body: await readBody(req) });
}
