import Image from 'next/image';

import { ABOUT } from '@/lib/content';
import { palette } from '@/lib/palette';

/** О бренде — the pull-quote, the QOIM chip, and the slogan/region pair. */
export function AboutBrand() {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '36px 32px 56px' }}>
      <div
        className="skc-2col"
        style={{
          borderTop: `1px solid ${palette.hairline}`,
          borderBottom: `1px solid ${palette.hairline}`,
          padding: '56px 0',
          display: 'grid',
          gridTemplateColumns: '1fr 1.1fr',
          gap: 56,
          alignItems: 'center',
        }}
      >
        <div>
          <p
            style={{
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '.18em',
              textTransform: 'uppercase',
              color: palette.primary,
              margin: '0 0 16px',
            }}
          >
            {ABOUT.eyebrow}
          </p>
          <blockquote
            className="disp"
            style={{
              fontSize: 30,
              lineHeight: 1.2,
              fontWeight: 700,
              color: palette.deep,
              margin: '0 0 20px',
            }}
          >
            {ABOUT.quoteLead}
            <span style={{ color: palette.primary }}>{ABOUT.quoteAccent}</span>.
          </blockquote>
          <p style={{ fontSize: 16, lineHeight: 1.6, color: palette.muted, margin: '0 0 22px' }}>
            {ABOUT.body}
          </p>
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 14,
              background: '#fff',
              border: `1px solid ${palette.hairline}`,
              borderRadius: 14,
              padding: '10px 18px 10px 10px',
            }}
          >
            <Image
              src="/assets/logo-qoim.png"
              alt=""
              width={52}
              height={52}
              style={{ height: 52, width: 52, objectFit: 'cover', borderRadius: 9, display: 'block' }}
            />
            <span>
              <span style={{ display: 'block', fontSize: 12.5, color: '#7C8C7E' }}>
                {ABOUT.entityLabel}
              </span>
              <span style={{ display: 'block', fontSize: 15, fontWeight: 700, color: palette.deep }}>
                {ABOUT.entity}
              </span>
            </span>
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
          <div
            style={{
              background: palette.primary,
              color: palette.section,
              borderRadius: 16,
              padding: 24,
            }}
          >
            <div
              style={{
                fontSize: 11,
                letterSpacing: '.12em',
                textTransform: 'uppercase',
                opacity: 0.85,
                marginBottom: 12,
              }}
            >
              {ABOUT.sloganLabel}
            </div>
            <div
              className="disp"
              style={{ fontSize: 20, fontWeight: 700, color: '#fff', lineHeight: 1.25 }}
            >
              {ABOUT.sloganTg}
            </div>
            <div style={{ fontSize: 12.5, opacity: 0.9, marginTop: 8 }}>{ABOUT.sloganRu}</div>
          </div>

          <div
            style={{
              background: '#fff',
              border: `1px solid ${palette.hairline}`,
              borderRadius: 16,
              padding: 24,
            }}
          >
            <div
              style={{
                fontSize: 11,
                letterSpacing: '.12em',
                textTransform: 'uppercase',
                color: '#7C8C7E',
                marginBottom: 12,
              }}
            >
              {ABOUT.regionLabel}
            </div>
            <div style={{ fontSize: 17, fontWeight: 700, color: palette.deep, lineHeight: 1.3 }}>
              {ABOUT.regionCity}
            </div>
            <div style={{ fontSize: 12.5, color: '#7C8C7E', marginTop: 8 }}>
              {ABOUT.regionArea}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
