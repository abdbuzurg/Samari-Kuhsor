import { getTranslations, setRequestLocale } from 'next-intl/server';

import { AssemblyLine } from '@/components/AssemblyLine';
import { Section, SectionHead } from '@/components/Section';
import { AboutBrand } from '@/components/home/AboutBrand';
import { ClosingCTA } from '@/components/home/ClosingCTA';
import { EcoBadges } from '@/components/home/EcoBadges';
import { Hero } from '@/components/home/Hero';
import { NewsGrid } from '@/components/home/NewsGrid';
import { ProductionPreview } from '@/components/home/ProductionPreview';
import { RetailerMarquee } from '@/components/home/RetailerMarquee';
import { RidgelineDivider } from '@/components/home/RidgelineDivider';
import { TajikistanMap } from '@/components/home/TajikistanMap';
import { TrustStrip } from '@/components/home/TrustStrip';
import { loadCatalogue } from '@/lib/catalogue';
import { loadNews } from '@/lib/news';

/**
 * Главная — the approved design, section for section.
 *
 * The order is the design's and is load-bearing: the product line is the hero
 * element, the retailer strip that follows it is deliberately quieter so it does
 * not compete, and the map, eco and about blocks build the regional story before
 * the closing call to action asks for anything.
 *
 * Twelve sections:
 *   hero · trust strip · assembly line (+ quick-look modal) · retailer marquee ·
 *   animated map · eco badges · ridgeline · about · production preview · news ·
 *   closing CTA
 */
export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations();

  const [products, news] = await Promise.all([loadCatalogue(locale), loadNews(locale)]);

  return (
    <>
      <Hero
        products={products}
        ctaProducts={t('cta.viewProducts')}
        ctaProduction={t('cta.aboutProduction')}
      />

      <TrustStrip />

      <Section>
        <SectionHead
          eyebrow={t('home.catalogueEyebrow')}
          title={t('home.catalogueTitle')}
          lead={t('home.catalogueLead')}
        />
        <AssemblyLine products={products} />
      </Section>

      <RetailerMarquee
        heading={t('home.retailersTitle')}
        caption={t('home.retailersCaption')}
      />

      <TajikistanMap />

      <EcoBadges />

      <RidgelineDivider />

      <AboutBrand />

      <ProductionPreview heading={t('home.processTitle')} more={t('cta.more')} />

      {/* Rendered only when there is news. An empty "Новости" heading above
          nothing reads as a broken page, and before launch there genuinely is
          none. */}
      {news.length > 0 && <NewsGrid heading={t('home.newsTitle')} items={news} />}

      <ClosingCTA />
    </>
  );
}
