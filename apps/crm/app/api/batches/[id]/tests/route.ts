import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/batches/${id}/tests`, { body: await readBody(req) });
}
