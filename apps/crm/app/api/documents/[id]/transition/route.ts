import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — moves a document along its approval ladder. Activation needs
 *  documents:approve; sending a draft for review needs no authority. Both rules
 *  are enforced in Go against the transition matrix. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/documents/${id}/transition`, { body: await readBody(req) });
}
