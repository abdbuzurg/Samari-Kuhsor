import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — moves a post along the ladder. Publishing needs cms:approve AND all
 *  three translations; both rules are Go's. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/cms/news/${id}/transition`, { body: await readBody(req) });
}
