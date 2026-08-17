import { getTranslations } from 'next-intl/server';

import { Section } from '@/components/Section';
import { Link } from '@/i18n/routing';
import { palette } from '@/lib/palette';

/** 404. Offers a way back rather than a dead end. */
export default async function NotFound() {
  const t = await getTranslations('nav');

  return (
    <Section>
      <h1 className="disp" style={{ fontSize: 34, margin: '0 0 12px', fontWeight: 800 }}>
        Страница не найдена
      </h1>
      <p style={{ fontSize: 15.5, color: palette.muted, margin: '0 0 24px' }}>
        Возможно, ссылка устарела или страница была перемещена.
      </p>
      <Link
        href="/"
        style={{
          textDecoration: 'none',
          fontSize: 15,
          fontWeight: 700,
          background: palette.primary,
          color: '#fff',
          padding: '14px 24px',
          borderRadius: 11,
          display: 'inline-block',
        }}
      >
        {t('home')}
      </Link>
    </Section>
  );
}
