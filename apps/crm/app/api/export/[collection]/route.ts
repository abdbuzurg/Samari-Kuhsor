import { NextRequest } from 'next/server';

import { download } from '@/lib/api';

/**
 * GET /api/export/{collection} — CSV download.
 *
 * Guarded in Go on the collection's own module, declared as one static route per
 * collection so the permission is checked at boot rather than at runtime. The
 * BFF passes the query string through untouched, which is what makes the export
 * carry the same filter as the screen it was launched from. `download` rather
 * than `proxy`: proxy parses and re-encodes JSON, which would turn a CSV into a
 * JSON string of itself.
 */
export async function GET(req: NextRequest, ctx: { params: Promise<{ collection: string }> }) {
  const { collection } = await ctx.params;
  return download(req, `/export/${encodeURIComponent(collection)}`);
}
