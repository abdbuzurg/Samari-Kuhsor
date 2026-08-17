import type { ReactNode } from 'react';

import { palette } from '@/lib/palette';

/**
 * Typographic wrapper for the legal pages.
 *
 * A single measure-limited column: legal text is read, not scanned, and a
 * full-width paragraph at 1240px is unreadable.
 */
export function LegalBody({ children }: { children: ReactNode }) {
  return (
    <div
      className="legal-body"
      style={{ maxWidth: 720, fontSize: 15, lineHeight: 1.75, color: palette.muted }}
    >
      {children}
      <style>{`
        .legal-body h2 {
          font-size: 17px;
          font-weight: 700;
          color: ${palette.deep};
          margin: 32px 0 10px;
        }
        .legal-body h2:first-child { margin-top: 0; }
        .legal-body p { margin: 0 0 14px; }
      `}</style>
    </div>
  );
}
