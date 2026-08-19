import { TRUST_ITEMS } from '@/lib/content';
import { palette } from '@/lib/palette';

/** The four-column trust strip. Four, not three — see lib/content.ts. */
export function TrustStrip() {
  return (
    <section style={{ background: '#fff', borderBottom: `1px solid rgba(35,88,58,.1)` }}>
      <div
        className="skc-2col"
        style={{
          maxWidth: 1240,
          margin: '0 auto',
          padding: '0 32px',
          display: 'grid',
          gridTemplateColumns: 'repeat(4,1fr)',
        }}
      >
        {TRUST_ITEMS.map((item) => (
          <div
            key={item.title}
            data-testid="trust-item"
            style={{ padding: '28px 22px', borderLeft: '1px solid rgba(35,88,58,.08)' }}
          >
            <h2
              style={{
                fontSize: 13.5,
                fontWeight: 700,
                color: palette.primaryHover,
                margin: '0 0 7px',
              }}
            >
              {item.title}
            </h2>
            <p style={{ fontSize: 12.5, lineHeight: 1.45, color: '#7C8C7E', margin: 0 }}>
              {item.text}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
