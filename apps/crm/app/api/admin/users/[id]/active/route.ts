import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** PUT — deactivates or restores a user. Go refuses to deactivate the last
 *  account holding admin:manage, so the system cannot be locked out of itself. */
export async function PUT(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/admin/users/${id}/active`, { body: await readBody(req) });
}
