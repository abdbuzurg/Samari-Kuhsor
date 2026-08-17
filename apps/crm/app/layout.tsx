import type { Metadata } from 'next';
import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';

import { Providers } from '@/components/Providers';
import './globals.css';

export const metadata: Metadata = {
  title: 'Самари Кӯҳсор — CRM/ERP',
  description: 'CRM-ERP платформа QOIM',
};

/**
 * Fonts are self-hosted by next/font rather than @import-ed from Google, so the
 * CRM works on a box with no outbound internet and does not block first paint on
 * a third-party round trip.
 *
 * The pairing is deliberate and is the fix for docs/07-IMPLEMENTATION-PLAN.md C1:
 * Archivo has no Cyrillic subset, so every Russian string in the approved
 * prototype was silently falling back to system-ui and Tajik was unrenderable.
 * Browsers fall back per glyph, so Latin still renders in Archivo exactly as
 * approved and only the Cyrillic — which was never specified — changes.
 *
 * The font-family itself is set by --font-body/--font-heading in theme.css.
 */
import { Archivo, Golos_Text } from 'next/font/google';

const archivo = Archivo({
  subsets: ['latin', 'latin-ext'],
  weight: ['400', '600', '800'],
  variable: '--font-archivo',
  display: 'swap',
});

const golos = Golos_Text({
  subsets: ['cyrillic', 'cyrillic-ext', 'latin', 'latin-ext'],
  weight: ['400', '500', '600', '700', '800', '900'],
  variable: '--font-golos',
  display: 'swap',
});

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html lang={locale} className={`${archivo.variable} ${golos.variable}`}>
      <body>
        <NextIntlClientProvider locale={locale} messages={messages}>
          <Providers>{children}</Providers>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
