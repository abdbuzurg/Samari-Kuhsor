'use client';

import { useState } from 'react';
import Image from 'next/image';
import { useLocale, useTranslations } from 'next-intl';
import { usePathname as useRawPathname } from 'next/navigation';

import { Link, usePathname } from '@/i18n/routing';
import { locales, localeLabels, type Locale } from '@/i18n/config';
import { BRAND } from '@/lib/content';
import { palette } from '@/lib/palette';

/**
 * Sticky header, from the prototype (site-source.html:268).
 *
 * The nav is four items and the language switcher is ТҶ / РУ / EN in that
 * order — both are part of the visual contract (CLAUDE.md §5).
 *
 * Below 1024px the same items move into a drawer. The row needs about 1080px:
 * logo, four nav items, three locale links and the CTA. At 393px it was 1066px
 * wide and forced every page on the site to scroll sideways — the single
 * largest mobile defect, and present on every route.
 *
 * A drawer rather than a squeeze: shrinking the row to fit produces 13px tap
 * targets. Nothing is removed and nothing is reordered, so the contract holds —
 * this decides only when the items are visible.
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
  const [open, setOpen] = useState(false);

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
        className="site-bar"
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
          <span style={{ display: 'flex', flexDirection: 'column', lineHeight: 1 }}>
            <span className="disp" style={{ fontWeight: 800, fontSize: 18, color: palette.deep }}>
              {BRAND.name}
            </span>
            {/* "Roof of the World · Pamir" — the regional framing the design
                established. Not a translated string: it is a wordmark, and it
                reads the same in all three locales. */}
            <span
              style={{
                fontSize: 9,
                letterSpacing: '.34em',
                textTransform: 'uppercase',
                color: palette.primary,
                marginTop: 5,
              }}
            >
              {BRAND.subtitle}
            </span>
          </span>
        </Link>

        <nav
          className="site-nav-desktop"
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
          className="site-locale-desktop"
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
          className="site-cta-desktop"
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

        <button
          type="button"
          className="site-burger"
          aria-expanded={open}
          aria-controls="site-drawer"
          aria-label={t('a11y.mainNav')}
          data-testid="site-burger"
          onClick={() => setOpen((v) => !v)}
          style={{
            alignItems: 'center',
            justifyContent: 'center',
            width: 44,
            height: 44,
            flex: 'none',
            border: `1px solid ${palette.hairline}`,
            borderRadius: 10,
            background: 'transparent',
            color: palette.deep,
            cursor: 'pointer',
          }}
        >
          <svg viewBox="0 0 24 24" width={22} height={22} aria-hidden fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
            {open ? (
              <>
                <path d="M6 6l12 12" />
                <path d="M18 6L6 18" />
              </>
            ) : (
              <>
                <path d="M4 7h16" />
                <path d="M4 12h16" />
                <path d="M4 17h16" />
              </>
            )}
          </svg>
        </button>
      </div>

      {open && (
        <div
          id="site-drawer"
          className="site-drawer"
          data-testid="site-drawer"
          style={{
            borderTop: `1px solid ${palette.hairline}`,
            background: 'rgba(244,247,248,.98)',
            padding: '10px 18px 18px',
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
          }}
        >
          <nav
            aria-label={t('a11y.mainNav')}
            style={{ display: 'flex', flexDirection: 'column', gap: 2 }}
          >
            {NAV.map((item) => {
              const active =
                item.href === '/' ? pathname === '/' : pathname.startsWith(item.href);
              return (
                <Link
                  key={item.key}
                  href={item.href}
                  aria-current={active ? 'page' : undefined}
                  // Closing on navigation: a drawer left covering the page it
                  // just opened is the most common mobile-menu defect.
                  onClick={() => setOpen(false)}
                  style={{
                    textDecoration: 'none',
                    fontSize: 15,
                    fontWeight: 600,
                    padding: '12px 12px',
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
              gap: 6,
              marginTop: 8,
              paddingTop: 12,
              borderTop: `1px solid ${palette.hairline}`,
            }}
          >
            {locales.map((l) => {
              const active = l === locale;
              return (
                <Link
                  key={l}
                  href={pathname}
                  locale={l}
                  hrefLang={l}
                  aria-current={active ? 'true' : undefined}
                  onClick={() => setOpen(false)}
                  style={{
                    textDecoration: 'none',
                    fontSize: 13,
                    fontWeight: 700,
                    padding: '10px 16px',
                    borderRadius: 9,
                    color: active ? '#fff' : palette.deep,
                    background: active ? palette.primary : 'rgba(62,142,95,.10)',
                  }}
                >
                  {localeLabels[l]}
                </Link>
              );
            })}
          </div>

          <Link
            href="/contact"
            onClick={() => setOpen(false)}
            style={{
              textDecoration: 'none',
              textAlign: 'center',
              fontSize: 14,
              fontWeight: 700,
              background: palette.primary,
              color: '#fff',
              padding: '13px 17px',
              borderRadius: 10,
              marginTop: 10,
            }}
          >
            {t('cta.distributors')}
          </Link>
        </div>
      )}
      {/* rawPathname is read so the header re-renders on navigation in tests
          that mount it outside a router transition. */}
      <span hidden data-path={rawPathname} />
    </header>
  );
}
