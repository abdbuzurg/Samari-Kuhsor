'use server';

import { cookies } from 'next/headers';

import { LOCALE_COOKIE, isLocale } from '@/i18n/config';

/**
 * Persists the interface language.
 *
 * The CRM's locale is a user preference in a cookie rather than a URL segment
 * (docs/07-IMPLEMENTATION-PLAN.md I14). `tg` is validated here, so an unknown or
 * legacy `tj` value cannot reach next-intl and blank the interface (C2).
 */
export async function setLocale(value: string) {
  if (!isLocale(value)) return;
  const store = await cookies();
  store.set(LOCALE_COOKIE, value, {
    path: '/',
    maxAge: 60 * 60 * 24 * 365,
    sameSite: 'lax',
  });
}
