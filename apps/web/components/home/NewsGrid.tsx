import { palette } from '@/lib/palette';
import type { NewsCard } from '@/lib/news';

/**
 * Новости.
 *
 * The image area is a tinted placeholder saying "фото новости" in monospace —
 * deliberately obvious. The client has not supplied news photography, and a
 * stock image on a food producer's news card would imply a photograph of their
 * factory that is not one.
 */
export function NewsGrid({ heading, items }: { heading: string; items: NewsCard[] }) {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '0 32px 40px' }}>
      <h2
        className="disp"
        style={{
          fontSize: 32,
          lineHeight: 1.05,
          fontWeight: 800,
          margin: '0 0 28px',
          color: palette.deep,
        }}
      >
        {heading}
      </h2>
      <div
        className="skc-2col"
        style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 20 }}
      >
        {items.map((n) => (
          <article
            key={n.id}
            data-testid="news-card"
            style={{
              background: '#fff',
              border: `1px solid rgba(35,88,58,.11)`,
              borderRadius: 18,
              overflow: 'hidden',
            }}
          >
            <div
              aria-hidden
              style={{
                aspectRatio: '16/10',
                background: palette.section,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <span
                style={{
                  fontFamily: 'ui-monospace,Menlo,monospace',
                  fontSize: 11,
                  color: '#96a191',
                }}
              >
                фото новости
              </span>
            </div>
            <div style={{ padding: 18 }}>
              <p
                style={{
                  fontSize: 11,
                  fontWeight: 700,
                  letterSpacing: '.08em',
                  textTransform: 'uppercase',
                  color: palette.accent,
                  margin: '0 0 10px',
                }}
              >
                {[n.category, n.publishedOn].filter(Boolean).join(' · ')}
              </p>
              <h3
                style={{
                  fontSize: 15.5,
                  fontWeight: 700,
                  color: palette.deep,
                  lineHeight: 1.3,
                  margin: 0,
                }}
              >
                {n.title}
              </h3>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
