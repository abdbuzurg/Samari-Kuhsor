import type { MetadataRoute } from 'next';

import { siteUrl } from '@/lib/site';

/**
 * Rendered per request, not at build time.
 *
 * Next prerenders both robots.txt and the sitemap as static files by default,
 * which would freeze the site's address into the image. The deploy sequence
 * changes it twice WITHOUT rebuilding — bare IP for the client test, then the
 * real domain once DNS resolves — so a build-time value would serve the wrong
 * address for the whole first stage. This was a real defect: robots.txt served
 * `Allow: /` under a bare IP because it had been baked at build time.
 *
 * The cost is one trivial render per request for two rarely-fetched files.
 */
export const dynamic = 'force-dynamic';

/**
 * robots.txt.
 *
 * `/api/` is disallowed — the BFF is not content, and a crawler POSTing nothing
 * useful to the enquiry endpoint would burn the rate limit for real visitors.
 *
 * The whole site is disallowed while PUBLIC_SITE_URL still points at a bare IP. The client tests over an IP before the domain is registered, and a test
 * deployment indexed under an address that will stop resolving leaves dead
 * results in search for months.
 */
export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();
  const isProvisional = /^https?:\/\/(\d{1,3}\.){3}\d{1,3}(:\d+)?$/.test(base);

  if (isProvisional) {
    return { rules: [{ userAgent: '*', disallow: '/' }] };
  }

  return {
    rules: [{ userAgent: '*', allow: '/', disallow: '/api/' }],
    sitemap: `${base}/sitemap.xml`,
  };
}
