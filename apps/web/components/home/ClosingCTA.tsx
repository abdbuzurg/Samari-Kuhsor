import { Link } from '@/i18n/routing';
import { CTA } from '@/lib/content';
import { palette } from '@/lib/palette';

/**
 * The closing call to action.
 *
 * Note what the copy does NOT promise: it says formats, specifications and terms
 * are available "как только продукция будет готова к отгрузке" — once the
 * product is ready to ship. The factory has not opened, and an unqualified
 * "order now" would be a claim the client cannot honour.
 */
export function ClosingCTA() {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '24px 32px 84px' }}>
      <div
        className="skc-2col"
        style={{
          position: 'relative',
          overflow: 'hidden',
          background: `linear-gradient(120deg,${palette.primaryHover},${palette.primary})`,
          borderRadius: 22,
          padding: '52px 48px',
          display: 'grid',
          gridTemplateColumns: '1.2fr 1fr',
          gap: 44,
          alignItems: 'center',
          color: '#fff',
        }}
      >
        <svg
          viewBox="0 0 600 200"
          preserveAspectRatio="none"
          aria-hidden
          style={{
            position: 'absolute',
            right: 0,
            bottom: 0,
            width: '60%',
            height: '100%',
            opacity: 0.16,
            pointerEvents: 'none',
          }}
        >
          <path
            d="M0,200 L120,90 L220,140 L340,50 L440,120 L540,60 L600,110 L600,200 Z"
            fill="none"
            stroke="#fff"
            strokeWidth="2"
          />
        </svg>

        <div style={{ position: 'relative' }}>
          <h2
            className="disp"
            style={{ fontSize: 32, lineHeight: 1.1, fontWeight: 800, margin: '0 0 14px' }}
          >
            {CTA.title}
          </h2>
          <p
            style={{
              fontSize: 17,
              lineHeight: 1.5,
              margin: '0 0 26px',
              maxWidth: 440,
              opacity: 0.95,
            }}
          >
            {CTA.body}
          </p>
          <Link
            href="/contact"
            style={{
              textDecoration: 'none',
              fontSize: 15,
              fontWeight: 700,
              background: '#fff',
              color: palette.deep,
              padding: '15px 26px',
              borderRadius: 11,
              display: 'inline-block',
            }}
          >
            {CTA.button}
          </Link>
        </div>

        <div
          style={{
            position: 'relative',
            display: 'grid',
            gridTemplateColumns: '1fr 1fr',
            gap: 14,
          }}
        >
          {CTA.tiles.map((tile) => (
            <div key={tile.label} style={tileStyle}>
              <div className="disp" style={{ fontSize: 24, fontWeight: 800 }}>
                {tile.value}
              </div>
              <div style={{ fontSize: 12.5, opacity: 0.9, marginTop: 4 }}>{tile.label}</div>
            </div>
          ))}
          <div style={{ ...tileStyle, gridColumn: 'span 2' }}>
            <div style={{ fontSize: 12.5, opacity: 0.9 }}>{CTA.wideTile.label}</div>
            <div style={{ fontSize: 15, fontWeight: 700, marginTop: 4 }}>
              {CTA.wideTile.value}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

const tileStyle: React.CSSProperties = {
  background: 'rgba(255,255,255,.15)',
  border: '1px solid rgba(255,255,255,.3)',
  borderRadius: 14,
  padding: 20,
};
