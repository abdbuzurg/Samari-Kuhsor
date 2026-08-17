import { Golos_Text } from 'next/font/google';
import { NextIntlClientProvider, hasLocale } from 'next-intl';
import { setRequestLocale, getTranslations } from 'next-intl/server';
import { notFound } from 'next/navigation';
import type { Metadata } from 'next';
import type { ReactNode } from 'react';

import { SiteHeader } from '@/components/SiteHeader';
import { SiteFooter } from '@/components/SiteFooter';
import { ConsentBanner } from '@/components/ConsentBanner';
import { Analytics } from '@/components/Analytics';
import { htmlLang, locales, type Locale } from '@/i18n/config';
import { routing } from '@/i18n/routing';
import { palette } from '@/lib/palette';
import { siteUrl } from '@/lib/site';

/**
 * Golos Text, self-hosted through next/font.
 *
 * Not substitutable and not loaded from a CDN. It was chosen because it renders
 * Tajik ҳ, ҷ and ӯ correctly (CLAUDE.md §5); next/font downloads and serves it
 * from our own origin, so the site has no third-party font request to consent
 * to and no render-blocking dependency on Google.
 */
const golos = Golos_Text({
  subsets: ['latin', 'cyrillic'],
  weight: ['400', '500', '600', '700', '800', '900'],
  variable: '--font-golos',
  display: 'swap',
});

export function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  if (!hasLocale(routing.locales, locale)) return {};
  const t = await getTranslations({ locale, namespace: 'home' });

  // hreflang points at real, distinct URLs — which is the whole reason this app
  // routes its locales while the CRM does not (I14). x-default goes to Russian:
  // the site's audience is overwhelmingly Russian-reading, and pointing it at a
  // language nobody local uses would be a worse guess than picking the default.
  const languages: Record<string, string> = {};
  for (const l of locales) {
    languages[htmlLang[l]] = `${siteUrl()}/${l}`;
  }
  languages['x-default'] = `${siteUrl()}/ru`;

  return {
    metadataBase: new URL(siteUrl()),
    title: {
      default: 'Самари Кӯҳсор — QOIM LLC',
      template: '%s — Самари Кӯҳсор',
    },
    description: t('lead'),
    alternates: {
      canonical: `${siteUrl()}/${locale}`,
      languages,
    },
    openGraph: {
      siteName: 'Самари Кӯҳсор',
      locale: htmlLang[locale as Locale],
      type: 'website',
    },
  };
}

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!hasLocale(routing.locales, locale)) {
    notFound();
  }
  // Required for static rendering of a routed-locale segment.
  setRequestLocale(locale);

  const t = await getTranslations('a11y');

  return (
    <html lang={htmlLang[locale as Locale]} className={golos.variable}>
      <body style={{ margin: 0, background: palette.page, color: palette.deep }}>
        <NextIntlClientProvider>
          {/* First focusable element on the page. The prototype has no skip
              link, which leaves keyboard users tabbing the whole nav on every
              page — a defect, not a design decision to reproduce. */}
          <a href="#main" className="skip-link">
            {t('skip')}
          </a>
          <SiteHeader />
          <main id="main">{children}</main>
          <SiteFooter />
          <ConsentBanner />
          <Analytics />
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
