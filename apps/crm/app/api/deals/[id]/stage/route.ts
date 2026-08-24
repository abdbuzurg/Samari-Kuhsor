import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/**
 * POST /api/deals/{id}/stage — moves a deal.
 *
 * An action rather than a PATCH of `stage`: the move is guarded by a matrix and
 * writes an immutable event, neither of which a field update could express.
 */
export async function POST(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/deals/${id}/stage`, { body: await readBody(req) });
}
