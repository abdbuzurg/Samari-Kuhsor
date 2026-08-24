/**
 * Renders an RFC 3339 UTC timestamp in Dushanbe time.
 *
 * The API sends UTC and says so (docs/03-API-CONTRACT.md:145); presenting it in
 * local time is explicitly the frontend's job. Asia/Dushanbe is fixed rather
 * than read from the browser: a director opening the CRM from abroad should see
 * the factory's clock, because that is when the thing actually happened.
 *
 * Extracted in R04 — the same function had been copied into two detail views,
 * and ten more were about to want it.
 */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return 'уточняется';
  return new Date(iso).toLocaleString('ru-RU', { timeZone: 'Asia/Dushanbe' });
}
