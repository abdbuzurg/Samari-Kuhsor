import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/cms/news/{id}/history — who moved this post along the ladder.
 *
 * The pages equivalent had a BFF route; news did not, so half the CMS workflow
 * history was unreachable. `content_workflow_events` is immutable evidence with
 * no version and no deleted_at — the record of who approved a public claim.
 */
export async function GET(req: NextRequest, ctx: { params: Promise<{ id: string }> }) {
  const { id } = await ctx.params;
  return proxy(req, `/cms/news/${id}/history`);
}
