import { NextRequest } from 'next/server';

import { download } from '@/lib/api';

/**
 * GET /api/batches/{id}/qr.svg — the rendered QR code.
 *
 * `download` rather than `proxy`: the response is an SVG document, and proxy
 * parses and re-encodes JSON. Without this route the batch screen could show the
 * payload as a string but nobody could see the code that gets printed — which is
 * the one thing worth checking before ordering wrappers against it (D11).
 */
export async function GET(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return download(req, `/batches/${id}/qr.svg`);
}
