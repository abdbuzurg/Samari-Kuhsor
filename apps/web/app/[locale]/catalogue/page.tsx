import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { CatalogueGrid } from '@/components/CatalogueGrid';
import { Section, SectionHead } from '@/components/Section';
import { loadCatalogue } from '@/lib/catalogue';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'catalogue' });
  return { title: t('title'), description: t('lead') };
}

/** Продукция — the catalogue page. */
export default async function CataloguePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('catalogue');
  const products = await loadCatalogue(locale);

  return (
    <Section>
      <SectionHead title={t('title')} lead={t('lead')} />
      <CatalogueGrid products={products} />
    </Section>
  );
}
