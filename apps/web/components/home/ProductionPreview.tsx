import { Link } from '@/i18n/routing';
import { PROCESS_STEPS } from '@/lib/content';
import { palette } from '@/lib/palette';

/**
 * The six process steps, titles only.
 *
 * The descriptions live on the Производство page. Same source list, so the two
 * cannot disagree about how many steps there are or what they are called.
 *
 * The 1px grid gap over a tinted background is how the design draws the cell
 * borders — one rule per seam rather than doubled borders between neighbours.
 */
export function ProductionPreview({ heading, more }: { heading: string; more: string }) {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '0 32px 44px' }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-end',
          gap: 24,
          flexWrap: 'wrap',
          marginBottom: 30,
        }}
      >
        <h2
          className="disp"
          style={{
            fontSize: 32,
            lineHeight: 1.05,
            fontWeight: 800,
            margin: 0,
            color: palette.deep,
          }}
        >
          {heading}
        </h2>
        <Link
          href="/production"
          style={{
            textDecoration: 'none',
            fontSize: 14,
            fontWeight: 700,
            color: palette.primaryHover,
            borderBottom: `2px solid ${palette.accent}`,
            paddingBottom: 3,
          }}
        >
          {more}
        </Link>
      </div>

      <ol
        className="skc-2col"
        style={{
          listStyle: 'none',
          margin: 0,
          padding: 0,
          display: 'grid',
          gridTemplateColumns: 'repeat(3,1fr)',
          gap: 1,
          background: palette.hairline,
          border: `1px solid ${palette.hairline}`,
          borderRadius: 16,
          overflow: 'hidden',
        }}
      >
        {PROCESS_STEPS.map((step) => (
          <li key={step.n} data-testid="process-step" style={{ background: '#fff', padding: '26px 24px' }}>
            <div
              className="disp"
              style={{ fontSize: 22, fontWeight: 800, color: palette.primary, marginBottom: 14 }}
            >
              {step.n}
            </div>
            <div
              style={{ fontSize: 15.5, fontWeight: 700, color: palette.deep, lineHeight: 1.3 }}
            >
              {step.title}
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
