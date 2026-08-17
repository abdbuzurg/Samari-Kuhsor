import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/admin/users/${id}/roles`, { body: await readBody(req) });
}
