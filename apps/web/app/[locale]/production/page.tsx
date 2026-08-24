import type { Metadata } from 'next';
import { getTranslations, setRequestLocale } from 'next-intl/server';

import { Section } from '@/components/Section';
import { Link } from '@/i18n/routing';
import { PROCESS_STEPS } from '@/lib/content';
import { palette } from '@/lib/palette';

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: 'production' });
  return { title: t('title'), description: t('lead') };
}

/**
 * Производство и качество.
 *
 * Rebuilt against the approved design (`Samari Kuhsor - C.dc.html`, §5.6 of the
 * design project's PROJECT-CONTEXT.md) after a comparison found three ways this
 * page had drifted from it:
 *
 *   1. It declared its OWN six steps rather than using PROCESS_STEPS, so steps
 *      02 and 03 were «Мойка и подготовка» / «Переработка» here and
 *      «Переработка фруктов и овощей» / «Водоподготовка и выдув ПЭТ» on the home
 *      page. lib/content.ts says in as many words that the two views "cannot
 *      disagree about how many steps there are or what they are called". They
 *      did.
 *   2. The steps were a six-across row of separate cards; the design is a 3×2
 *      grid inside one bordered card with hairline dividers.
 *   3. The closing card — «Гигиена, прослеживаемость и лаборатория» with its
 *      three tags and a photo placeholder — had been replaced by a different
 *      «Контроль качества» section.
 *
 * The steps make no claim about certification, because nothing is certified yet:
 * the product pages say «Статус сертификации: на согласовании» and this page
 * must not contradict them.
 */
export default async function ProductionPage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations('production');

  return (
    <>
      {/* The tinted header band with a ridgeline, from the design. The built
          page had a plain SectionHead on the page background. */}
      <div
        style={{
          background: `linear-gradient(180deg, ${palette.section} 0%, ${palette.page} 100%)`,
          borderBottom: `1px solid ${palette.hairline}`,
        }}
      >
        <Section>
          <nav
            aria-label={t('breadcrumbHome')}
            style={{ fontSize: 12.5, color: palette.muted, marginBottom: 14 }}
          >
            <Link href="/" style={{ textDecoration: 'none' }}>
              {t('breadcrumbHome')}
            </Link>
            <span aria-hidden> / </span>
            <span>{t('title')}</span>
          </nav>
          <h1
            className="disp skc-display-xl"
            style={{ fontSize: 52, lineHeight: 1.06, margin: 0, fontWeight: 800 }}
          >
            {t('title')}
          </h1>
          <p
            style={{
              fontSize: 16,
              lineHeight: 1.62,
              color: palette.muted,
              margin: '14px 0 0',
              maxWidth: 620,
            }}
          >
            {t('lead')}
          </p>
        </Section>
      </div>

      <Section>
        {/* One card, 3×2, hairline dividers between cells — the design's shape.
            .skc-2col collapses it to a single column below 760px. */}
        <ol
          className="skc-2col skc-steps"
          style={{
            listStyle: 'none',
            margin: 0,
            padding: 0,
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
            background: '#fff',
            border: `1px solid ${palette.hairline}`,
            borderRadius: 18,
            overflow: 'hidden',
          }}
        >
          {PROCESS_STEPS.map((s, i) => (
            <li
              key={s.n}
              style={{
                padding: '26px 26px 30px',
                // Dividers rather than gaps: the design reads as one card cut
                // into six, not six cards side by side.
                borderRight: (i + 1) % 3 === 0 ? 'none' : `1px solid ${palette.hairline}`,
                borderBottom: i < 3 ? `1px solid ${palette.hairline}` : 'none',
              }}
            >
              <span
                className="disp"
                style={{
                  display: 'block',
                  fontSize: 26,
                  fontWeight: 800,
                  color: palette.primary,
                  marginBottom: 12,
                }}
              >
                {s.n}
              </span>
              <h2 style={{ fontSize: 15.5, lineHeight: 1.35, margin: '0 0 8px', fontWeight: 700 }}>
                {s.title}
              </h2>
              <p style={{ fontSize: 13.5, lineHeight: 1.55, color: palette.muted, margin: 0 }}>
                {s.text}
              </p>
            </li>
          ))}
        </ol>
      </Section>

      <Section>
        <div
          className="skc-2col"
          style={{
            display: 'grid',
            gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)',
            gap: 34,
            alignItems: 'center',
            background: '#fff',
            border: `1px solid ${palette.hairline}`,
            borderRadius: 20,
            padding: '38px 38px 42px',
          }}
        >
          <div>
            <h2
              className="disp skc-display-lg"
              style={{ fontSize: 25, lineHeight: 1.2, margin: '0 0 14px', fontWeight: 800 }}
            >
              {t('closingTitle')}
            </h2>
            <p style={{ fontSize: 14.5, lineHeight: 1.65, color: palette.muted, margin: '0 0 20px' }}>
              {t('closingBody')}
            </p>
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              {[t('tagHaccp'), t('tagTraceability'), t('tagLab')].map((tag) => (
                <span
                  key={tag}
                  style={{
                    fontSize: 12.5,
                    fontWeight: 700,
                    color: palette.deep,
                    background: palette.section,
                    borderRadius: 999,
                    padding: '8px 15px',
                  }}
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>

          {/* Real photography of the hall has to come from the client
              (PROJECT-CONTEXT.md §10). The placeholder is the design's own. */}
          <div
            style={{
              background: palette.section,
              borderRadius: 16,
              minHeight: 300,
              display: 'grid',
              placeItems: 'center',
              fontSize: 12.5,
              color: palette.muted,
            }}
          >
            {t('photoPlaceholder')}
          </div>
        </div>
      </Section>
    </>
  );
}
