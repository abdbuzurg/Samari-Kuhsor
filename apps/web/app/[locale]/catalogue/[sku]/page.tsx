import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { ProductViewTracker } from '@/components/ProductViewTracker';
import { Section } from '@/components/Section';
import { Link } from '@/i18n/routing';
import { CATALOGUE_ORDER, loadCatalogue } from '@/lib/catalogue';
import { specsFor } from '@/lib/specs';
import { locales } from '@/i18n/config';
import { palette } from '@/lib/palette';

/**
 * Prerender the approved five.
 *
 * Built from the constant order rather than from a live API call: a build that
 * fails because the backend is briefly down would block a deploy over a
 * catalogue that has not changed in weeks. A sixth product added later is still
 * served — the route stays dynamic for anything not in this list.
 */
export function generateStaticParams() {
  return locales.flatMap((locale) =>
    CATALOGUE_ORDER.map((sku) => ({ locale, sku })),
  );
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string; sku: string }>;
}): Promise<Metadata> {
  const { locale, sku } = await params;
  const product = (await loadCatalogue(locale)).find((p) => p.sku === sku);
  if (!product) return {};
  return { title: product.name, description: product.description };
}

/**
 * One product page.
 *
 * The specification table is the reason this page exists, and most of it reads
 * «Уточняется». That is deliberate and was the client's explicit instruction
 * (docs/02-SCHEMA.md:176): composition, nutritional values, shelf life and water
 * classification are not published until the recipes are approved and the lab
 * has tested them. An empty cell would read as "none"; «Уточняется» is the truth.
 */
export default async function ProductPage({
  params,
}: {
  params: Promise<{ locale: string; sku: string }>;
}) {
  const { locale, sku } = await params;
  setRequestLocale(locale);
  const t = await getTranslations();

  const product = (await loadCatalogue(locale)).find((p) => p.sku === sku);
  if (!product) notFound();

  const specs = specsFor(product);

  return (
    <Section>
      {/* The other surface that shows a product. A visitor arriving here from
          search never clicked anything, so a click-only scheme would make every
          well-ranking product look unpopular (docs/01-DECISIONS.md D12). */}
      <ProductViewTracker sku={product.sku} source="product_page" />
      <Link
        href="/catalogue"
        style={{
          textDecoration: 'none',
          fontSize: 13.5,
          fontWeight: 700,
          color: palette.primaryHover,
          display: 'inline-block',
          marginBottom: 24,
        }}
      >
        {t('product.back')}
      </Link>

      <div
        className="skc-2col"
        style={{ display: 'grid', gridTemplateColumns: '.82fr 1.18fr', gap: 44 }}
      >
        <div
          aria-hidden
          style={{
            background: product.tint,
            borderRadius: 18,
            minHeight: 320,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <span
            style={{
              width: 96,
              height: 190,
              borderRadius: '12px 12px 16px 16px',
              background: '#fff',
              borderTop: `24px solid ${product.accent}`,
              display: 'block',
            }}
          />
        </div>

        <div>
          <p
            style={{
              margin: '0 0 10px',
              fontSize: 12,
              fontWeight: 700,
              letterSpacing: '.05em',
              textTransform: 'uppercase',
              color: product.accent,
            }}
          >
            {product.line}
          </p>
          <h1
            className="disp"
            style={{ fontSize: 34, lineHeight: 1.15, margin: '0 0 16px', fontWeight: 800 }}
          >
            {product.name}
          </h1>
          <p style={{ fontSize: 15.5, lineHeight: 1.65, color: palette.muted, margin: '0 0 28px' }}>
            {product.description}
          </p>

          <h2 style={{ fontSize: 15, fontWeight: 700, margin: '0 0 12px' }}>
            {t('product.specs')}
          </h2>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13.5 }}>
            <tbody>
              {specs.map((row, i) => (
                <tr key={row.k} style={{ background: i % 2 ? palette.page : '#fff' }}>
                  <th
                    scope="row"
                    style={{
                      textAlign: 'left',
                      fontWeight: 600,
                      padding: '10px 14px',
                      width: '46%',
                      color: palette.muted,
                    }}
                  >
                    {row.k}
                  </th>
                  <td style={{ padding: '10px 14px' }}>{row.v}</td>
                </tr>
              ))}
            </tbody>
          </table>

          <div style={{ display: 'flex', gap: 12, marginTop: 26, flexWrap: 'wrap' }}>
            <Link
              href="/contact"
              style={{
                textDecoration: 'none',
                fontSize: 14.5,
                fontWeight: 700,
                background: palette.primary,
                color: '#fff',
                padding: '13px 22px',
                borderRadius: 11,
              }}
            >
              {t('cta.requestPrice')}
            </Link>
            <Link
              href="/production"
              style={{
                textDecoration: 'none',
                fontSize: 14.5,
                fontWeight: 700,
                color: palette.deep,
                padding: '13px 22px',
                borderRadius: 11,
                border: `1.5px solid ${palette.hairlineStrong}`,
              }}
            >
              {t('cta.learnMore')}
            </Link>
          </div>
        </div>
      </div>
    </Section>
  );
}
