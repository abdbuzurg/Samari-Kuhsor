import { getRequestConfig } from 'next-intl/server';
import { cookies } from 'next/headers';

import { LOCALE_COOKIE, defaultLocale, isLocale } from './config';

/**
 * Locale resolution for the CRM.
 *
 * Unrouted: the CRM is behind a login, so the interface language is a user
 * preference stored in a cookie rather than a URL segment
 * (docs/07-IMPLEMENTATION-PLAN.md I14). Putting `[locale]` in every route would
 * add a segment to all 13 modules for a benefit only the public site needs.
 *
 * This module imports next/headers and is therefore server-only. The shared
 * constants live in ./config so client components can use them.
 */
export default getRequestConfig(async () => {
  const store = await cookies();
  const cookieLocale = store.get(LOCALE_COOKIE)?.value;
  const locale = isLocale(cookieLocale) ? cookieLocale : defaultLocale;

  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
