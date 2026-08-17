import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** GET — the permission catalogue the role editor renders its checkboxes from.
 *  Generated in Go from rbac's own tables, so the editor cannot offer a
 *  permission the middleware does not recognise. */
export async function GET(req: NextRequest) {
  return proxy(req, '/admin/permissions');
}
