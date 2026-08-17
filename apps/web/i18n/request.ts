import { getRequestConfig } from 'next-intl/server';

import { routing } from './routing';
import { defaultLocale, isLocale } from './config';

export default getRequestConfig(async ({ requestLocale }) => {
  const requested = await requestLocale;
  // A locale that is not one of ours falls back rather than 404ing: a stale link
  // to /tj/... should show the site, not an error page.
  const locale = isLocale(requested) ? requested : defaultLocale;
  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});

export { routing };
