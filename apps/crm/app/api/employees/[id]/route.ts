import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function PATCH(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/employees/${id}`, { body: await readBody(req) });
}
