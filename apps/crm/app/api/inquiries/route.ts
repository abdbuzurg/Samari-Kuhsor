import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/** Интеграция с сайтом — docs/05-MODULES.md §11. */
export async function GET(req: NextRequest) {
  return proxy(req, '/inquiries');
}
