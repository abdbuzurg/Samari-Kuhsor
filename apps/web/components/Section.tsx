import type { ReactNode } from 'react';

import { palette } from '@/lib/palette';

/** The prototype's section wrapper: 1240px, 32px gutters. */
export function Section({ children }: { children: ReactNode }) {
  return (
    <section
      className="skc-section"
      style={{ maxWidth: 1240, margin: '0 auto', padding: '64px 32px 8px' }}
    >
      {children}
    </section>
  );
}

export function SectionHead({
  eyebrow,
  title,
  lead,
}: {
  /** Small tracked caps above the heading — «КАТАЛОГ», «ЧИСТОТА ПАМИРА». Part
   *  of the design's section rhythm; omitted, the headings read as a flat list. */
  eyebrow?: string;
  title: string;
  lead?: string;
}) {
  return (
    <div style={{ marginBottom: 24, maxWidth: 720 }}>
      {eyebrow && (
        <div
          style={{
            fontSize: 11,
            letterSpacing: '.18em',
            textTransform: 'uppercase',
            fontWeight: 700,
            color: palette.primary,
            marginBottom: 10,
          }}
        >
          {eyebrow}
        </div>
      )}
      <h2
        className="disp skc-display-lg"
        style={{ fontSize: 30, lineHeight: 1.15, margin: 0, fontWeight: 800 }}
      >
        {title}
      </h2>
      {lead && (
        <p style={{ fontSize: 15.5, lineHeight: 1.6, color: palette.muted, margin: '12px 0 0' }}>
          {lead}
        </p>
      )}
    </div>
  );
}
