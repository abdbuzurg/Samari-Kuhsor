import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/cms/pages/${id}/blocks`);
}

export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/cms/pages/${id}/blocks`, { body: await readBody(req) });
}
