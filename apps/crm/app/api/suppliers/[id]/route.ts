import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** GET /api/suppliers/{id} — the detail view's read. Guarded in Go. */
export async function GET(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/suppliers/${id}`);
}
