import { callApi, relay } from '@/lib/api';
import type { User } from '@samari/types';

/**
 * GET /api/auth/me — the caller's identity and resolved permissions.
 *
 * The permission list drives which nav entries and buttons the CRM renders. That
 * is presentation only: the server enforces every request, and a hidden button
 * is never a control (docs/04-RBAC.md:120).
 */
export async function GET() {
  return relay(await callApi<User>('/auth/me'));
}
