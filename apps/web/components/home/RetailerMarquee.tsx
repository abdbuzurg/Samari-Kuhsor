import { RETAILERS } from '@/lib/content';

/**
 * The retailer trust belt.
 *
 * A continuous marquee, deliberately quieter than the product line: no hover
 * detail, no click, no modal. The catalogue is the hero of this page and a
 * second interactive belt would compete with it.
 *
 * The list is duplicated so the CSS translate can loop seamlessly — the second
 * copy is aria-hidden, because a screen reader announcing eight retailer names
 * twice is noise, not information.
 *
 * The caption is not optional. These are placeholder names for a factory that
 * has not opened, and without the caption the strip reads as a claim about who
 * stocks the product.
 */
export function RetailerMarquee({ heading, caption }: { heading: string; caption: string }) {
  return (
    <>
      <section style={{ maxWidth: 1240, margin: '0 auto', padding: '52px 32px 8px' }}>
        <div style={{ textAlign: 'center', marginBottom: 22 }}>
          <h2
            style={{
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '.18em',
              textTransform: 'uppercase',
              color: '#7C8C7E',
              margin: 0,
            }}
          >
            {heading}
          </h2>
        </div>
      </section>

      <section
        style={{
          position: 'relative',
          overflow: 'hidden',
          padding: '6px 0 8px',
          WebkitMaskImage:
            'linear-gradient(90deg,transparent,#000 8%,#000 92%,transparent)',
          maskImage: 'linear-gradient(90deg,transparent,#000 8%,#000 92%,transparent)',
        }}
      >
        <div
          className="skc-marquee"
          data-testid="retailer-marquee"
          style={{ display: 'flex', gap: 20, width: 'max-content', padding: '0 10px' }}
        >
          {[0, 1].map((copy) =>
            RETAILERS.map((r) => (
              <div
                key={`${copy}-${r.name}`}
                className="skc-logo"
                aria-hidden={copy === 1 ? true : undefined}
                style={{
                  flex: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 9,
                  height: 52,
                  padding: '0 24px',
                  border: '1px solid rgba(35,88,58,.13)',
                  borderRadius: 12,
                  background: '#fff',
                }}
              >
                {/* Chorkhona diamond — the Pamiri skylight motif, used as a light
                    accent only (CLAUDE.md §5). */}
                <svg viewBox="0 0 24 24" aria-hidden style={{ width: 18, height: 18, flex: 'none' }}>
                  <rect
                    x="6"
                    y="6"
                    width="12"
                    height="12"
                    transform="rotate(45 12 12)"
                    fill="none"
                    stroke={r.color}
                    strokeWidth="1.5"
                  />
                </svg>
                <span
                  className="disp"
                  style={{
                    fontSize: 17,
                    fontWeight: 800,
                    letterSpacing: '.02em',
                    color: r.color,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {r.name}
                </span>
              </div>
            )),
          )}
        </div>
        <p
          style={{ textAlign: 'center', fontSize: 11, color: '#96a191', margin: '16px 0 0' }}
          data-testid="retailer-caption"
        >
          {caption}
        </p>
      </section>
    </>
  );
}
