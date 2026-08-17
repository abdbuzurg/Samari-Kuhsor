import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/admin/roles/${id}`, { body: await readBody(req) });
}
