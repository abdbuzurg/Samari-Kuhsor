import { ECO_BADGES, ECO_SECTION } from '@/lib/content';
import { palette } from '@/lib/palette';

/**
 * The three purity badges.
 *
 * Each sits in a chorkhona-style frame — the nested-square Pamiri skylight motif
 * — with a hand-drawn icon. All of this is vector work from the approved design;
 * there is no photography in this workflow and none should be substituted.
 */

const EG = '#2E7A4B';
const EA = palette.accent;

const TINTS: Record<string, string> = {
  leaf: 'rgba(62,142,95,.10)',
  drop: 'rgba(63,163,174,.12)',
  mtn: 'rgba(231,154,58,.13)',
};

/** Shared stroke defaults, so the three icons read as one set. */
function stroke(width = 2.2, colour = EG) {
  return {
    fill: 'none',
    stroke: colour,
    strokeWidth: width,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
  };
}

function EcoIcon({ kind }: { kind: string }) {
  const tint = TINTS[kind] ?? TINTS.leaf;
  return (
    <svg viewBox="0 0 60 60" aria-hidden style={{ width: 72, height: 72, display: 'block' }}>
      <rect
        x={0.6}
        y={0.6}
        width={58.8}
        height={58.8}
        rx={15}
        fill="#F7F1E6"
        stroke="rgba(35,88,58,.14)"
        strokeWidth={1}
      />
      <circle cx={30} cy={30} r={19} fill={tint} />

      {/* Без консервантов → a leaf: nothing artificial added. */}
      {kind === 'leaf' && (
        <>
          <path d="M30 13 C 19 21, 18 35, 30 47 C 42 35, 41 21, 30 13 Z" {...stroke()} />
          <path d="M30 18 L30 44" {...stroke(1.9)} />
          <path d="M30 27 L23.5 23" {...stroke(1.7)} />
          <path d="M30 27 L36.5 23" {...stroke(1.7)} />
          <path d="M30 35 L22.5 31" {...stroke(1.7)} />
          <path d="M30 35 L37.5 31" {...stroke(1.7)} />
        </>
      )}

      {/* Без добавок → a droplet with an approving check: clean composition. */}
      {kind === 'drop' && (
        <>
          <path
            d="M30 13 C 23 25, 19 31, 19 37 A 11 11 0 0 0 41 37 C 41 31, 37 25, 30 13 Z"
            {...stroke()}
          />
          <path d="M25 37 l3.6 3.6 L36 33" {...stroke(2.4, EA)} />
        </>
      )}

      {/* Эко-производство → Pamir peaks with a rising sprout. */}
      {kind === 'mtn' && (
        <>
          <path d="M14 42 L25 27 L31 35 L37 26 L46 42" {...stroke()} />
          <path d="M12 42 L48 42" {...stroke(1.9)} />
          <path d="M25 27 L22 22.5 L25.5 21" {...stroke(1.9, EA)} />
          <circle cx={39} cy={20} r={3.4} fill="none" stroke={EA} strokeWidth={1.9} />
        </>
      )}
    </svg>
  );
}

export function EcoBadges() {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '48px 32px 8px' }}>
      <div style={{ textAlign: 'center', maxWidth: 720, margin: '0 auto 36px' }}>
        <p
          style={{
            fontSize: 11,
            fontWeight: 700,
            letterSpacing: '.18em',
            textTransform: 'uppercase',
            color: palette.primary,
            margin: '0 0 14px',
          }}
        >
          {ECO_SECTION.eyebrow}
        </p>
        <h2
          className="disp"
          style={{
            fontSize: 34,
            lineHeight: 1.12,
            fontWeight: 800,
            margin: '0 0 14px',
            color: palette.deep,
          }}
        >
          {ECO_SECTION.title}
        </h2>
        <p style={{ fontSize: 16, lineHeight: 1.6, color: palette.muted, margin: 0 }}>
          {ECO_SECTION.lead}
        </p>
      </div>

      <div
        className="skc-2col"
        style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 20 }}
      >
        {ECO_BADGES.map((badge) => (
          <div
            key={badge.title}
            data-testid="eco-badge"
            style={{
              background: '#fff',
              border: `1px solid rgba(35,88,58,.11)`,
              borderRadius: 18,
              padding: '32px 28px',
              textAlign: 'center',
            }}
          >
            <div style={{ width: 72, height: 72, margin: '0 auto 20px' }}>
              <EcoIcon kind={badge.icon} />
            </div>
            <h3 style={{ fontSize: 17, fontWeight: 800, color: palette.deep, margin: '0 0 8px' }}>
              {badge.title}
            </h3>
            <p style={{ fontSize: 13, lineHeight: 1.5, color: '#7C8C7E', margin: 0 }}>
              {badge.text}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
