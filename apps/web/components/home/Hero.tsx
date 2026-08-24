import { Link } from '@/i18n/routing';
import { HERO } from '@/lib/content';
import { palette } from '@/lib/palette';
import type { PublicProduct } from '@/lib/catalogue';

/**
 * The hero, from the approved design.
 *
 * The layered ridgelines behind it are three SVG paths at increasing opacity —
 * the "mountain line-art" motif that runs through the whole site. They are
 * decorative and marked aria-hidden: a screen reader announcing three unnamed
 * paths adds nothing.
 */
export function Hero({
  products,
  ctaProducts,
  ctaProduction,
}: {
  products: PublicProduct[];
  ctaProducts: string;
  ctaProduction: string;
}) {
  return (
    <section
      style={{
        position: 'relative',
        overflow: 'hidden',
        background: `linear-gradient(180deg,${palette.section} 0%,${palette.page} 100%)`,
        borderBottom: `1px solid rgba(35,88,58,.1)`,
      }}
    >
      <svg
        viewBox="0 0 1440 320"
        preserveAspectRatio="none"
        aria-hidden
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          width: '100%',
          height: 320,
          pointerEvents: 'none',
        }}
      >
        <path
          d="M0,320 L0,150 L200,96 L400,140 L620,82 L840,134 L1060,90 L1280,138 L1440,104 L1440,320 Z"
          fill="rgba(62,142,95,.05)"
        />
        <path
          d="M0,320 L0,206 L240,162 L470,200 L700,150 L940,200 L1180,160 L1440,194 L1440,320 Z"
          fill="rgba(62,142,95,.08)"
        />
        <path
          d="M0,320 L0,256 L260,226 L520,258 L780,220 L1040,258 L1300,228 L1440,250 L1440,320 Z"
          fill="rgba(62,142,95,.13)"
          stroke="rgba(62,142,95,.32)"
          strokeWidth="1"
        />
      </svg>

      <div
        className="skc-hero"
        style={{
          position: 'relative',
          maxWidth: 1240,
          margin: '0 auto',
          padding: '80px 32px 78px',
          display: 'grid',
          gridTemplateColumns: '1.12fr .88fr',
          gap: 52,
          alignItems: 'center',
        }}
      >
        <div>
          <p
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 10,
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '.2em',
              textTransform: 'uppercase',
              color: palette.primaryHover,
              background: palette.section,
              padding: '8px 15px',
              borderRadius: 30,
              margin: '0 0 28px',
            }}
          >
            <svg viewBox="0 0 24 24" aria-hidden style={{ width: 13, height: 13 }}>
              <path
                d="M2 20 L8 9 L12 14 L16 6 L22 20 Z"
                fill="none"
                stroke={palette.primaryHover}
                strokeWidth="1.6"
                strokeLinejoin="round"
              />
            </svg>
            {HERO.eyebrow}
          </p>

          <h1
            className="disp skc-display-xl"
            style={{
              fontSize: 60,
              lineHeight: 1.02,
              fontWeight: 800,
              margin: '0 0 24px',
              color: palette.deep,
            }}
          >
            {HERO.titleLead}
            <br />
            {HERO.titleAccentPrefix}
            <span style={{ color: palette.primary }}>{HERO.titleAccent}</span>
          </h1>

          <p
            style={{
              fontSize: 18,
              lineHeight: 1.6,
              color: palette.muted,
              margin: '0 0 34px',
              maxWidth: 500,
            }}
          >
            {HERO.lead}
          </p>

          <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap' }}>
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
              {ctaProducts}
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
              {ctaProduction}
            </Link>
          </div>
        </div>

        {/* The product list card. Four rows in the design; the fifth product is
            reachable from the catalogue. */}
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
          {/* All five, not four. The catalogue is exactly five products (D8) and
              the design lists every one of them; slicing to four dropped
              «Питьевая вода 1 л» off the hero entirely.

              Row shape is the design's: index, colour bar, name over
              «volume · packaging», arrow. The built version had the index on the
              right, no arrow, and «line · volume» — so the packaging, which is
              what a wholesale buyer is actually scanning for, never appeared. */}
          {products.map((p, i) => (
            <li key={p.id}>
              <Link
                href={`/catalogue/${p.sku}`}
                style={{
                  textDecoration: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 16,
                  padding: '16px 20px',
                  borderBottom:
                    i === products.length - 1 ? 'none' : '1px solid rgba(35,88,58,.08)',
                }}
              >
                <span
                  className="disp"
                  aria-hidden
                  style={{
                    fontSize: 13,
                    fontWeight: 800,
                    color: palette.muted,
                    flex: 'none',
                    minWidth: 20,
                  }}
                >
                  {p.idx}
                </span>
                <span
                  aria-hidden
                  style={{
                    width: 6,
                    height: 34,
                    borderRadius: 3,
                    background: p.accent,
                    flex: 'none',
                  }}
                />
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'block', fontSize: 14, fontWeight: 700 }}>
                    {p.short}
                  </span>
                  <span style={{ display: 'block', fontSize: 12, color: palette.muted }}>
                    {p.volume} · {p.pack}
                  </span>
                </span>
                <span aria-hidden style={{ fontSize: 14, color: palette.muted, flex: 'none' }}>
                  →
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
