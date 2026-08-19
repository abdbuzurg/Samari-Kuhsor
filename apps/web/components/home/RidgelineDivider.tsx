/**
 * The ridgeline section divider.
 *
 * A layered mountain range: back range, front range, snow caps on the three
 * tallest front peaks, and a crisp stroke along the front ridgeline. It replaced
 * an earlier abstract zigzag that read as a squiggle rather than as mountains.
 *
 * Purely decorative, so aria-hidden — it separates sections visually and carries
 * no information a screen reader needs.
 */
export function RidgelineDivider() {
  return (
    <div style={{ maxWidth: 1240, margin: '0 auto', padding: '40px 32px 0' }}>
      <svg
        viewBox="0 0 1200 120"
        preserveAspectRatio="none"
        aria-hidden
        data-testid="ridgeline"
        style={{ width: '100%', height: 78, display: 'block' }}
      >
        <path
          d="M0,120 L0,74 L120,44 L220,66 L340,30 L470,60 L600,26 L740,58 L870,36 L1000,62 L1110,42 L1200,60 L1200,120 Z"
          fill="#DCE4E1"
        />
        <path
          d="M0,120 L0,96 L150,58 L250,84 L360,50 L430,70 L520,40 L560,52 L620,44 L700,78 L820,54 L960,86 L1080,60 L1200,88 L1200,120 Z"
          fill="#C4D1CE"
        />
        <path d="M520,40 L502,54 L512,54 L520,47 L528,55 L539,55 Z" fill="#F4F7F5" />
        <path d="M360,50 L343,66 L353,65 L360,58 L369,66 L379,64 Z" fill="#F4F7F5" />
        <path d="M150,58 L134,76 L145,74 L150,67 L158,75 L167,73 Z" fill="#F4F7F5" />
        <path
          d="M0,96 L150,58 L250,84 L360,50 L430,70 L520,40 L560,52 L620,44 L700,78 L820,54 L960,86 L1080,60 L1200,88"
          fill="none"
          stroke="#9DB0AC"
          strokeWidth="1.2"
          strokeLinejoin="round"
        />
      </svg>
    </div>
  );
}
