import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

export async function PUT(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/tasks/${id}/status`, { body: await readBody(req) });
}
