import { NextRequest } from 'next/server';

import { proxy } from '@/lib/api';

/**
 * GET /api/dashboard — Панель управления.
 *
 * Each panel in the response is null when the caller may not read the module
 * behind it — decided in Go, never here. Null and zero are different answers and
 * the frontend must be able to tell them apart.
 */
export async function GET(req: NextRequest) {
  return proxy(req, '/dashboard');
}
