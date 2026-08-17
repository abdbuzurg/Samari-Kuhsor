import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { ContactForm } from '@/components/ContactForm';
import { Section, SectionHead } from '@/components/Section';
import { palette } from '@/lib/palette';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'contact' });
  return { title: t('title'), description: t('lead') };
}

export default async function ContactPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('contact');

  return (
    <Section>
      <SectionHead title={t('title')} lead={t('lead')} />
      <div
        className="skc-2col"
        style={{ display: 'grid', gridTemplateColumns: '.9fr 1.1fr', gap: 44, alignItems: 'start' }}
      >
        <div>
          <h2 style={{ fontSize: 15, fontWeight: 700, margin: '0 0 10px' }}>QOIM LLC</h2>
          <address
            style={{
              fontStyle: 'normal',
              fontSize: 14.5,
              lineHeight: 1.7,
              color: palette.muted,
              margin: 0,
            }}
          >
            {t('address')}
          </address>
        </div>
        <ContactForm />
      </div>
    </Section>
  );
}
