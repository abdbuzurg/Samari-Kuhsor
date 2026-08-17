import { defineRouting } from 'next-intl/routing';
import { createNavigation } from 'next-intl/navigation';

import { defaultLocale, locales } from './config';

/**
 * Routing for the public site.
 *
 * `localePrefix: 'always'` — every page has an explicit locale in its URL,
 * including Russian. The alternative, hiding the default locale's prefix, gives
 * the same content two addresses (`/` and `/ru`) and needs a canonical tag to
 * stop them competing. An explicit prefix costs one redirect from `/` and
 * removes the ambiguity entirely.
 */
export const routing = defineRouting({
  locales,
  defaultLocale,
  localePrefix: 'always',
});

export const { Link, redirect, usePathname, useRouter, getPathname } =
  createNavigation(routing);
