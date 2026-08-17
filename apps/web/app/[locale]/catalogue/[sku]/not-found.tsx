import { getTranslations } from 'next-intl/server';

import { Section } from '@/components/Section';
import { Link } from '@/i18n/routing';
import { palette } from '@/lib/palette';

/** A product that does not exist. Sends the visitor to the catalogue rather than
 *  the home page — they were looking for a product. */
export default async function ProductNotFound() {
  const t = await getTranslations('catalogue');

  return (
    <Section>
      <h1 className="disp" style={{ fontSize: 30, margin: '0 0 12px', fontWeight: 800 }}>
        Продукт не найден
      </h1>
      <p style={{ fontSize: 15, color: palette.muted, margin: '0 0 24px' }}>
        Возможно, ссылка устарела. Посмотрите весь ассортимент.
      </p>
      <Link
        href="/catalogue"
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
        {t('title')}
      </Link>
    </Section>
  );
}
