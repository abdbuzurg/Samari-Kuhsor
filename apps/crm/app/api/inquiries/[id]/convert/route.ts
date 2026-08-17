import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/** POST — converts an enquiry to a lead, carrying the reference number across so
 *  the trail from website to order is unbroken. */
export async function POST(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return proxy(req, `/inquiries/${id}/convert`, { body: await readBody(req) });
}
