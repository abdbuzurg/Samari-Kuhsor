import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

export async function GET(req: NextRequest) {
  return proxy(req, '/cms/media');
}
