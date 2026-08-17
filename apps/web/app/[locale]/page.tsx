import { getTranslations, setRequestLocale } from 'next-intl/server';

import { AssemblyLine } from '@/components/AssemblyLine';
import { Section, SectionHead } from '@/components/Section';
import { Link } from '@/i18n/routing';
import { loadCatalogue } from '@/lib/catalogue';
import { loadNews } from '@/lib/news';
import { palette } from '@/lib/palette';

/** Главная — the prototype's home page (site-source.html:294). */
export default async function HomePage({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations();

  const [products, news] = await Promise.all([loadCatalogue(locale), loadNews(locale)]);

  return (
    <>
      {/* Hero */}
      <section
        style={{
          position: 'relative',
          overflow: 'hidden',
          background: `linear-gradient(180deg,${palette.section} 0%,${palette.page} 100%)`,
          borderBottom: `1px solid ${palette.hairline}`,
        }}
      >
        <div
          className="skc-hero"
          style={{
            maxWidth: 1240,
            margin: '0 auto',
            padding: '72px 32px 84px',
            display: 'grid',
            gridTemplateColumns: '1.05fr .95fr',
            gap: 48,
            alignItems: 'center',
          }}
        >
          <div>
            <p
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 7,
                fontSize: 12.5,
                fontWeight: 700,
                letterSpacing: '.06em',
                textTransform: 'uppercase',
                color: palette.primaryHover,
                margin: '0 0 18px',
              }}
            >
              {t('home.kicker')}
            </p>
            <h1
              className="disp"
              style={{ fontSize: 46, lineHeight: 1.08, margin: '0 0 20px', fontWeight: 800 }}
            >
              {t('home.title')}
            </h1>
            <p style={{ fontSize: 16.5, lineHeight: 1.65, color: palette.muted, margin: '0 0 30px' }}>
              {t('home.lead')}
            </p>
            <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
              <Link
                href="/catalogue"
                style={{
                  textDecoration: 'none',
                  fontSize: 15,
                  fontWeight: 700,
                  background: palette.primary,
                  color: '#fff',
                  padding: '15px 26px',
                  borderRadius: 11,
                }}
              >
                {t('cta.viewProducts')}
              </Link>
              <Link
                href="/production"
                style={{
                  textDecoration: 'none',
                  fontSize: 15,
                  fontWeight: 700,
                  color: palette.deep,
                  padding: '15px 26px',
                  borderRadius: 11,
                  border: `1.5px solid ${palette.hairlineStrong}`,
                }}
              >
                {t('cta.aboutProduction')}
              </Link>
            </div>
          </div>

          <ul
            style={{
              listStyle: 'none',
              margin: 0,
              padding: 0,
              background: '#fff',
              borderRadius: 18,
              border: `1px solid ${palette.hairline}`,
              overflow: 'hidden',
            }}
          >
            {products.slice(0, 4).map((p) => (
              <li key={p.id}>
                <Link
                  href={`/catalogue/${p.sku}`}
                  style={{
                    textDecoration: 'none',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 18,
                    padding: '18px 22px',
                    borderBottom: `1px solid rgba(35,88,58,.08)`,
                  }}
                >
                  <span
                    aria-hidden
                    style={{
                      width: 34,
                      height: 46,
                      borderRadius: '6px 6px 8px 8px',
                      background: p.tint,
                      borderTop: `10px solid ${p.accent}`,
                      flex: 'none',
                    }}
                  />
                  <span style={{ flex: 1 }}>
                    <span style={{ display: 'block', fontSize: 14.5, fontWeight: 700 }}>
                      {p.short}
                    </span>
                    <span style={{ display: 'block', fontSize: 12.5, color: palette.muted }}>
                      {p.line} · {p.volume}
                    </span>
                  </span>
                  <span style={{ fontSize: 12, color: palette.muted, fontWeight: 700 }}>
                    {p.idx}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </section>

      {/* Trust strip */}
      <section style={{ background: '#fff', borderBottom: `1px solid ${palette.hairline}` }}>
        <div
          className="skc-2col"
          style={{
            maxWidth: 1240,
            margin: '0 auto',
            padding: '30px 32px',
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
            gap: 28,
          }}
        >
          {[
            {
              title: 'Лабораторный контроль',
              text: 'Проверки на этапах производства до публикации заявлений.',
            },
            {
              title: 'Прослеживаемость',
              text: 'Контроль сырья от приёмки до готовой продукции.',
            },
            {
              title: 'Местное сырьё',
              text: 'Фрукты и вода Памира, переработанные рядом с местом сбора.',
            },
          ].map((item) => (
            <div key={item.title}>
              <h2 style={{ fontSize: 14.5, fontWeight: 700, margin: '0 0 6px' }}>{item.title}</h2>
              <p style={{ fontSize: 13.5, lineHeight: 1.6, color: palette.muted, margin: 0 }}>
                {item.text}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* Catalogue */}
      <Section>
        <SectionHead title={t('home.catalogueTitle')} lead={t('home.catalogueLead')} />
        <AssemblyLine products={products} />
      </Section>

      {/* News */}
      {news.length > 0 && (
        <Section>
          <SectionHead title={t('home.newsTitle')} />
          <div
            className="skc-2col"
            style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 20 }}
          >
            {news.map((n) => (
              <article
                key={n.id}
                style={{
                  background: '#fff',
                  borderRadius: 16,
                  border: `1px solid ${palette.hairline}`,
                  overflow: 'hidden',
                }}
              >
                <div style={{ height: 8, background: palette.section }} aria-hidden />
                <div style={{ padding: '20px 22px 24px' }}>
                  <p
                    style={{
                      margin: '0 0 10px',
                      fontSize: 12,
                      fontWeight: 700,
                      letterSpacing: '.05em',
                      textTransform: 'uppercase',
                      color: palette.primaryHover,
                    }}
                  >
                    {n.category ?? ''} {n.publishedOn ? `· ${n.publishedOn}` : ''}
                  </p>
                  <h3 style={{ margin: 0, fontSize: 16, lineHeight: 1.35, fontWeight: 700 }}>
                    {n.title}
                  </h3>
                  {n.excerpt && (
                    <p
                      style={{
                        margin: '10px 0 0',
                        fontSize: 13.5,
                        lineHeight: 1.6,
                        color: palette.muted,
                      }}
                    >
                      {n.excerpt}
                    </p>
                  )}
                </div>
              </article>
            ))}
          </div>
        </Section>
      )}
    </>
  );
}
