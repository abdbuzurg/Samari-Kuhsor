import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** GET — the traceability view: the batch, its tests, its decision history and
 *  where its stock currently sits (docs/05-MODULES.md §7). */
export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/batches/${id}/detail`);
}
