import 'server-only';

import { callApi } from '@/lib/api';
import type { PublicNewsItem } from '@samari/types';

export interface NewsCard {
  id: string;
  slug: string;
  category: string | null;
  publishedOn: string | null;
  title: string;
  excerpt: string | null;
}

/**
 * Published news for the home page.
 *
 * Returns an empty list rather than throwing when the API is unreachable or has
 * nothing published. The home page renders the news section only when there is
 * news — an empty "Новости" heading above nothing reads as a broken page, and
 * before launch there genuinely is no news to show.
 */
export async function loadNews(locale: string, limit = 3): Promise<NewsCard[]> {
  const result = await callApi<PublicNewsItem[]>(
    `/public/news?locale=${encodeURIComponent(locale)}&limit=${limit}`,
    { revalidate: 300 },
  );
  if (!result.ok || !Array.isArray(result.data)) return [];

  return result.data.map((n) => ({
    id: n.id,
    slug: n.slug,
    category: n.category ?? null,
    publishedOn: n.published_on ?? null,
    title: n.title,
    excerpt: n.excerpt ?? null,
  }));
}
