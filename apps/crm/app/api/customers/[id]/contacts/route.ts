import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function POST(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/customers/${id}/contacts`, { body: await readBody(req) });
}
