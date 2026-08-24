import { NextRequest } from 'next/server';

import { download } from '@/lib/api';

/**
 * GET /api/batches/qr-export — the printer handoff, as a ZIP.
 *
 * `download` rather than `proxy`: the response is a binary archive, and proxy
 * parses and re-encodes JSON. Go buffers the ZIP rather than streaming it, so a
 * mid-stream failure cannot hand the printer a truncated archive that looks
 * complete — a short export means a batch ships with the wrong wrapper.
 */
export async function GET(req: NextRequest) {
  return download(req, '/batches/qr-export');
}
