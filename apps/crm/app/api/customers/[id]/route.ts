import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function GET(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/customers/${id}`);
}

export async function PATCH(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/customers/${id}`, { body: await readBody(req) });
}
