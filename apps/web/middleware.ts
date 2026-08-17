import createMiddleware from 'next-intl/middleware';

import { routing } from './i18n/routing';

export default createMiddleware(routing);

export const config = {
  // Everything except Next internals, the BFF, and static files. `/api` is
  // excluded deliberately: a locale prefix on an API route would be meaningless
  // and would break the BFF's paths.
  matcher: ['/((?!api|_next|_vercel|assets|.*\\..*).*)'],
};
