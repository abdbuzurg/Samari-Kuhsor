import type { ReactNode } from 'react';

import { palette } from '@/lib/palette';

/** The prototype's section wrapper: 1240px, 32px gutters. */
export function Section({ children }: { children: ReactNode }) {
  return (
    <section style={{ maxWidth: 1240, margin: '0 auto', padding: '64px 32px 8px' }}>
      {children}
    </section>
  );
}

export function SectionHead({ title, lead }: { title: string; lead?: string }) {
  return (
    <div style={{ marginBottom: 24, maxWidth: 720 }}>
      <h2 className="disp" style={{ fontSize: 30, lineHeight: 1.15, margin: 0, fontWeight: 800 }}>
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
