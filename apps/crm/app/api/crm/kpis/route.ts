import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** The four KPIs specified at docs/05-MODULES.md:179. */
export async function GET(req: NextRequest) {
  return proxy(req, '/crm/kpis');
}
