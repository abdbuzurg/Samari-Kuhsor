import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * POST /api/batches/{id}/qr — issues the QR payload.
 *
 * Once issued the payload never changes, because wrappers are ordered against
 * it (D11). Go enforces that; this route only carries the request.
 */
export async function POST(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/batches/${id}/qr`);
}
