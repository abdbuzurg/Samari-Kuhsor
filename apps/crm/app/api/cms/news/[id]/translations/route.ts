import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/cms/news/${id}/translations`);
}

export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/cms/news/${id}/translations`, { body: await readBody(req) });
}
