import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

export async function GET(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/batches/${id}`);
}
