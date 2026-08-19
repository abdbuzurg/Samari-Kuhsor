import Image from 'next/image';
import { getTranslations } from 'next-intl/server';

import { Link } from '@/i18n/routing';
import { FOOTER } from '@/lib/content';
import { palette } from '@/lib/palette';

/**
 * The footer, four columns as in the approved design.
 *
 * One deliberate departure. The design renders the legal items as plain `<span>`
 * labels, because the prototype had no pages behind them — its own handoff lists
 * "legal pages need to be written" as outstanding development work. Those pages
 * now exist, so these are real links. Shipping a public site whose privacy
 * policy is an unclickable word would be reproducing a placeholder rather than
 * the design.
 *
 * The Компания column stays as labels: О компании, Таджикистан, Новости и медиа
 * and Загрузки have no pages yet, and a link to a 404 is worse than plain text.
 */
export async function SiteFooter() {
  const t = await getTranslations();

  return (
    <footer style={{ background: palette.deep, color: '#96a191' }}>
      <div
        className="skc-2col"
        style={{
          maxWidth: 1240,
          margin: '0 auto',
          padding: '52px 32px 30px',
          display: 'grid',
          gridTemplateColumns: '1.6fr 1fr 1fr 1fr',
          gap: 32,
        }}
      >
        <div>
          <div
            style={{
              display: 'inline-flex',
              background: '#fff',
              borderRadius: 14,
              padding: 12,
              marginBottom: 14,
            }}
          >
            <Image
              src="/assets/logo-samari-mark.png"
              alt=""
              width={46}
              height={46}
              style={{ height: 46, width: 'auto', display: 'block' }}
            />
          </div>
          <div
            className="disp"
            style={{ fontWeight: 800, fontSize: 18, color: '#fff', marginBottom: 6 }}
          >
            {FOOTER.brand}
          </div>
          <div
            style={{
              fontSize: 10,
              letterSpacing: '.3em',
              textTransform: 'uppercase',
              color: '#7FBF8C',
              marginBottom: 16,
            }}
          >
            {FOOTER.tagline}
          </div>
          <p
            style={{
              fontSize: 13,
              lineHeight: 1.6,
              maxWidth: 300,
              margin: '0 0 16px',
            }}
          >
            {FOOTER.blurb}
          </p>
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 11,
              background: 'rgba(255,255,255,.06)',
              border: '1px solid rgba(255,255,255,.12)',
              borderRadius: 12,
              padding: '8px 14px 8px 8px',
            }}
          >
            <Image
              src="/assets/logo-qoim.png"
              alt=""
              width={40}
              height={40}
              style={{ height: 40, width: 40, objectFit: 'cover', borderRadius: 8, display: 'block' }}
            />
            <span style={{ fontSize: 12.5, color: '#cfe0d3' }}>
              {FOOTER.companyChip} <b style={{ color: '#fff' }}>{FOOTER.entity}</b>
            </span>
          </div>
        </div>

        <nav aria-label={FOOTER.sectionsLabel}>
          <h2 style={columnHeading}>{FOOTER.sectionsLabel}</h2>
          <div style={columnList}>
            <Link href="/" style={footerLink}>
              {t('nav.home')}
            </Link>
            <Link href="/catalogue" style={footerLink}>
              {t('nav.catalogue')}
            </Link>
            <Link href="/production" style={footerLink}>
              {t('nav.production')}
            </Link>
            <Link href="/contact" style={footerLink}>
              {t('nav.contact')}
            </Link>
          </div>
        </nav>

        <div>
          <h2 style={columnHeading}>{FOOTER.companyLabel}</h2>
          <div style={columnList}>
            {FOOTER.companyLinks.map((label) => (
              <span key={label}>{label}</span>
            ))}
          </div>
        </div>

        <nav aria-label={FOOTER.legalLabel}>
          <h2 style={columnHeading}>{FOOTER.legalLabel}</h2>
          <div style={columnList}>
            <Link href="/privacy" style={footerLink}>
              {t('footer.privacy')}
            </Link>
            <Link href="/terms" style={footerLink}>
              {t('footer.terms')}
            </Link>
          </div>
        </nav>
      </div>

      <div style={{ borderTop: '1px solid rgba(255,255,255,.12)' }}>
        <div
          style={{
            maxWidth: 1240,
            margin: '0 auto',
            padding: '18px 32px',
            display: 'flex',
            justifyContent: 'space-between',
            gap: 16,
            flexWrap: 'wrap',
            fontSize: 12,
            color: '#6f9678',
          }}
        >
          <span>{FOOTER.copyright}</span>
          <span>{FOOTER.slogan}</span>
        </div>
      </div>
    </footer>
  );
}

const columnHeading: React.CSSProperties = {
  fontSize: 12,
  letterSpacing: '.08em',
  textTransform: 'uppercase',
  color: '#6f9678',
  margin: '0 0 14px',
  fontWeight: 400,
};

const columnList: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 9,
  fontSize: 13.5,
};

const footerLink: React.CSSProperties = { textDecoration: 'none' };
