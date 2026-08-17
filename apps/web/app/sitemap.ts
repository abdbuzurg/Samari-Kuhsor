import type { MetadataRoute } from 'next';

import { locales } from '@/i18n/config';
import { CATALOGUE_ORDER } from '@/lib/catalogue';
import { siteUrl } from '@/lib/site';

/**
 * The sitemap.
 *
 * Every page appears once per locale, each entry carrying the alternates for the
 * other two. That is what tells a crawler the three URLs are the same page in
 * different languages rather than three thin duplicates — the same job hreflang
 * does in the document head, stated where the crawler looks first.
 *
 * Product URLs come from the approved SKU list rather than from a live API call:
 * a sitemap that 500s when the backend is briefly down is worse than one that is
 * occasionally a product behind, and the catalogue is exactly five products
 * (docs/01-DECISIONS.md).
 */
export default function sitemap(): MetadataRoute.Sitemap {
  const base = siteUrl();
  const paths = [
    '',
    '/catalogue',
    '/production',
    '/contact',
    '/privacy',
    '/terms',
    ...CATALOGUE_ORDER.map((sku) => `/catalogue/${sku}`),
  ];

  return paths.flatMap((path) =>
    locales.map((locale) => ({
      url: `${base}/${locale}${path}`,
      changeFrequency: 'weekly' as const,
      priority: path === '' ? 1 : 0.7,
      alternates: {
        languages: Object.fromEntries(
          locales.map((other) => [other, `${base}/${other}${path}`]),
        ),
      },
    })),
  );
}
