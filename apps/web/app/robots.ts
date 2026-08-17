import type { MetadataRoute } from 'next';

import { siteUrl } from '@/lib/site';

/**
 * robots.txt.
 *
 * `/api/` is disallowed — the BFF is not content, and a crawler POSTing nothing
 * useful to the enquiry endpoint would burn the rate limit for real visitors.
 *
 * The whole site is disallowed while NEXT_PUBLIC_SITE_URL still points at a bare
 * IP. The client tests over an IP before the domain is registered, and a test
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
