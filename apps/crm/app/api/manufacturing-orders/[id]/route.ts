import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/manufacturing-orders/${id}`);
}
