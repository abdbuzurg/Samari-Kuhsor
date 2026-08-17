import { NextRequest } from 'next/server';

import { proxy, readBody } from '@/lib/api';

/**
 * POST /api/inquiries — the contact form.
 *
 * The only write the public site makes, and the only unauthenticated write in
 * the system. Everything that protects it is in Go: type and length validation,
 * the batch check for complaints, and a rate limit keyed on the visitor's IP
 * (docs/03-API-CONTRACT.md:249).
 *
 * The BFF adds nothing but the service key and the forwarded IP. Validating here
 * as well would create a second copy of the rules that drifts from the first,
 * and it would not be enforcement anyway — anything reaching this route has
 * already been shaped by whoever sent it.
 */
export async function POST(req: NextRequest) {
  return proxy(req, '/public/inquiries', { body: await readBody(req) });
}
