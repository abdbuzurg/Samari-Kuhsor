import Image from 'next/image';
import { getTranslations } from 'next-intl/server';

import { Link } from '@/i18n/routing';
import { palette } from '@/lib/palette';

/** Footer, from the prototype. A server component: nothing here is interactive. */
export async function SiteFooter() {
  const t = await getTranslations();
  const year = new Date().getFullYear();

  return (
    <footer
      style={{
        background: palette.deep,
        color: '#EAF1DD',
        marginTop: 64,
      }}
    >
      <div
        style={{
          maxWidth: 1240,
          margin: '0 auto',
          padding: '48px 32px 32px',
          display: 'grid',
          gap: 32,
          gridTemplateColumns: 'minmax(220px, 1fr) auto',
          alignItems: 'start',
        }}
      >
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 14 }}>
            <Image
              src="/assets/logo-qoim.png"
              alt=""
              width={36}
              height={36}
              style={{ display: 'block', height: 36, width: 'auto' }}
            />
            <span style={{ fontWeight: 800, fontSize: 15 }}>{t('company')}</span>
          </div>
          <p style={{ fontSize: 13.5, lineHeight: 1.6, opacity: 0.85, margin: 0, maxWidth: 420 }}>
            {t('contact.address')}
          </p>
        </div>

        <nav style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Link href="/privacy" style={{ fontSize: 13.5, textDecoration: 'none', opacity: 0.85 }}>
            {t('footer.privacy')}
          </Link>
          <Link href="/terms" style={{ fontSize: 13.5, textDecoration: 'none', opacity: 0.85 }}>
            {t('footer.terms')}
          </Link>
        </nav>
      </div>

      <div
        style={{
          borderTop: '1px solid rgba(234,241,221,.15)',
          padding: '18px 32px',
          fontSize: 12.5,
          opacity: 0.7,
        }}
      >
        <div style={{ maxWidth: 1240, margin: '0 auto' }}>
          © {year} {t('company')} · {t('brand')}. {t('footer.rights')}
        </div>
      </div>
    </footer>
  );
}
