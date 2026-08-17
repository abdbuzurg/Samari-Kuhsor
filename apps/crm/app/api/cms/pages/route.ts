import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** CMS — docs/05-MODULES.md §15. The public site renders only `published`; the
 *  CRM can preview any state, which is why these are separate endpoints from
 *  the website's /public/* reads. */
export async function GET(req: NextRequest) {
  return proxy(req, '/cms/pages');
}
