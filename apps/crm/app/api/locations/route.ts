import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** GET /api/locations — warehouse zones, for the movement form's pickers. */
export async function GET(req: NextRequest) {
  return proxy(req, '/locations');
}
