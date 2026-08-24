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
 * The three project milestones the approved design ships with.
 *
 * The same pattern as the catalogue's FALLBACK, and for the same reason: the
 * home page has a Новости section in the design, and a build or a deploy with an
 * empty CMS must still render the page the client signed off on.
 *
 * An earlier version returned an empty list and the home page hid the section
 * entirely, on the reasoning that "before launch there genuinely is no news".
 * That is true of the CMS and not of the design — the client wrote these three
 * cards themselves, and the section is simply missing from the site compared to
 * what they approved.
 *
 * Anything published through the CMS replaces these completely. They are a
 * floor, not a merge.
 */
const FALLBACK: NewsCard[] = [
  {
    id: 'fallback-construction',
    slug: 'stroitelstvo-ploshchadki',
    category: 'Строительство',
    publishedOn: '2026-06-01',
    title: 'Строительство производственной площадки в Теме',
    excerpt: null,
  },
  {
    id: 'fallback-equipment',
    slug: 'montazh-oborudovaniya',
    category: 'Оборудование',
    publishedOn: '2026-07-01',
    title: 'Монтаж технологического оборудования',
    excerpt: null,
  },
  {
    id: 'fallback-launch',
    slug: 'podgotovka-k-zapusku',
    category: 'Запуск',
    publishedOn: '2026-08-01',
    title: 'Подготовка к запуску четырёх продуктовых линий',
    excerpt: null,
  },
];

/**
 * Published news for the home page.
 *
 * Falls back to the approved three when the API is unreachable or has nothing
 * published, so the section the design specifies is always on the page.
 */
export async function loadNews(locale: string, limit = 3): Promise<NewsCard[]> {
  const result = await callApi<PublicNewsItem[]>(
    `/public/news?locale=${encodeURIComponent(locale)}&limit=${limit}`,
    { revalidate: 300 },
  );
  if (!result.ok || !Array.isArray(result.data) || result.data.length === 0) {
    return FALLBACK.slice(0, limit);
  }

  return result.data.map((n) => ({
    id: n.id,
    slug: n.slug,
    category: n.category ?? null,
    publishedOn: n.published_on ?? null,
    title: n.title,
    excerpt: n.excerpt ?? null,
  }));
}
