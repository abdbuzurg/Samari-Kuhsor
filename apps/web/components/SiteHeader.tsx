'use client';

import Image from 'next/image';
import { useLocale, useTranslations } from 'next-intl';
import { usePathname as useRawPathname } from 'next/navigation';

import { Link, usePathname } from '@/i18n/routing';
import { locales, localeLabels, type Locale } from '@/i18n/config';
import { palette } from '@/lib/palette';

/**
 * Sticky header, from the prototype (site-source.html:268).
 *
 * The nav is four items and the language switcher is ТҶ / РУ / EN in that
 * order — both are part of the visual contract (CLAUDE.md §5).
 */

const NAV = [
  { key: 'home', href: '/' },
  { key: 'catalogue', href: '/catalogue' },
  { key: 'production', href: '/production' },
  { key: 'contact', href: '/contact' },
] as const;

export function SiteHeader() {
  const t = useTranslations();
  const locale = useLocale() as Locale;
  const pathname = usePathname();
  const rawPathname = useRawPathname();

  return (
    <header
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 40,
        background: 'rgba(244,247,248,.86)',
        backdropFilter: 'blur(12px)',
        borderBottom: `1px solid ${palette.hairline}`,
      }}
    >
      <div
        style={{
          maxWidth: 1240,
          margin: '0 auto',
          padding: '14px 32px',
          display: 'flex',
          alignItems: 'center',
          gap: 12,
        }}
      >
        <Link
          href="/"
          style={{
            textDecoration: 'none',
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            flex: 'none',
          }}
        >
          <Image
            src="/assets/logo-samari-mark.png"
            alt=""
            width={38}
            height={38}
            priority
            style={{ display: 'block', height: 38, width: 'auto' }}
          />
          <span style={{ fontWeight: 800, fontSize: 16, letterSpacing: '-.02em' }}>
            {t('brand')}
          </span>
        </Link>

        <nav
          aria-label={t('a11y.mainNav')}
          style={{ display: 'flex', gap: 4, marginLeft: 'auto' }}
        >
          {NAV.map((item) => {
            // The catalogue tab stays active on a product page: a visitor who
            // drilled into one product has not left the section.
            const active =
              item.href === '/'
                ? pathname === '/'
                : pathname.startsWith(item.href);
            return (
              <Link
                key={item.key}
                href={item.href}
                aria-current={active ? 'page' : undefined}
                style={{
                  textDecoration: 'none',
                  fontSize: 13.5,
                  fontWeight: 600,
                  padding: '8px 13px',
                  borderRadius: 9,
                  color: active ? palette.deep : palette.muted,
                  background: active ? 'rgba(62,142,95,.13)' : 'transparent',
                }}
              >
                {t(`nav.${item.key}`)}
              </Link>
            );
          })}
        </nav>

        <div
          role="group"
          aria-label={t('a11y.language')}
          style={{
            display: 'flex',
            gap: 2,
            marginLeft: 8,
            paddingLeft: 12,
            borderLeft: `1px solid ${palette.hairline}`,
          }}
        >
          {locales.map((l) => {
            const active = l === locale;
            return (
              <Link
                key={l}
                // Switching language keeps you on the same page rather than
                // dropping you on the home page — losing your place is the
                // single most common complaint about language switchers.
                href={pathname}
                locale={l}
                hrefLang={l}
                aria-current={active ? 'true' : undefined}
                style={{
                  textDecoration: 'none',
                  fontSize: 12,
                  fontWeight: 700,
                  padding: '4px 9px',
                  borderRadius: 7,
                  color: active ? '#fff' : palette.deep,
                  background: active ? palette.primary : 'transparent',
                }}
              >
                {localeLabels[l]}
              </Link>
            );
          })}
        </div>

        <Link
          href="/contact"
          style={{
            textDecoration: 'none',
            fontSize: 13,
            fontWeight: 700,
            background: palette.primary,
            color: '#fff',
            padding: '10px 17px',
            borderRadius: 9,
            whiteSpace: 'nowrap',
            flex: 'none',
          }}
        >
          {t('cta.distributors')}
        </Link>
      </div>
      {/* rawPathname is read so the header re-renders on navigation in tests
          that mount it outside a router transition. */}
      <span hidden data-path={rawPathname} />
    </header>
  );
}
