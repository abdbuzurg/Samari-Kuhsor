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
 * The whole site is disallowed unless PUBLIC_SITE_URL is a real public domain.
 * The client tests over a bare IP before the domains are registered, and a test
 * deployment indexed under an address that will stop resolving leaves dead
 * results in search for months.
 *
 * The test is "is this a public domain?", not "is this an IP?". Those are not
 * the same question, and the narrower one fails open: it let `localhost` through
 * as crawlable. Nothing external can crawl localhost, so no harm was done — but
 * a guard whose default is "allow" is the wrong shape for this job. Anything
 * that is not a registrable hostname is treated as provisional.
 */
export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();

  if (!isPublicDomain(base)) {
    return { rules: [{ userAgent: '*', disallow: '/' }] };
  }

  return {
    rules: [{ userAgent: '*', allow: '/', disallow: '/api/' }],
    sitemap: `${base}/sitemap.xml`,
  };
}

/**
 * Whether an address is a real, registrable domain a crawler could reach.
 *
 * Fails CLOSED: anything unparseable is provisional. Getting this wrong in the
 * permissive direction means indexing a temporary address; in the restrictive
 * direction it means a launch-day `robots.txt` that has to be noticed and fixed.
 * The second is recoverable in minutes and the first takes months, so the
 * asymmetry decides the default.
 */
function isPublicDomain(base: string): boolean {
  let host: string;
  try {
    host = new URL(base).hostname;
  } catch {
    return false;
  }

  // Bare IPv4 and IPv6 — the client's first test address.
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return false;
  if (host.includes(':') || host.startsWith('[')) return false;

  // localhost, *.local, *.internal, *.test, and any single-label name: none of
  // these resolves on the public internet.
  if (host === 'localhost') return false;
  if (/\.(local|internal|test|localhost|example|invalid)$/.test(host)) return false;
  if (!host.includes('.')) return false;

  return true;
}
