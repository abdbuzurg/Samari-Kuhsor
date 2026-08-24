'use client';

import { useEffect } from 'react';
import type { ReactNode } from 'react';

/**
 * The shared shell for a printable document (R16).
 *
 * **Why the browser rather than server-side PDF generation.** The gate for this
 * task is that Cyrillic and Tajik `ҳ ҷ ӯ` render correctly. Every Go PDF library
 * requires embedding a TTF that covers those glyphs, and the project's font is
 * distributed as a subset woff2 loaded through next/font — converting and
 * re-embedding it is a font-licensing and glyph-coverage gamble whose failure
 * mode is silently shipping mojibake onto a regulatory document.
 *
 * A print route inherits the fonts the application already loads and proved.
 * "Печать → Сохранить как PDF" produces a correct, selectable, searchable PDF
 * with no new dependency and no second rendering path to keep in step with the
 * screen. The cost is that it cannot be generated unattended — which nothing in
 * the first release needs.
 *
 * No chrome: this renders on its own page with no sidebar and no top bar,
 * because a printed certificate carrying a navigation menu is not a document.
 */
export function PrintDocument({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  useEffect(() => {
    document.title = title;
  }, [title]);

  return (
    <div className="print-document">
      <style>{`
        @page { size: A4; margin: 18mm 16mm; }
        @media print {
          .print-hide { display: none !important; }
          .print-document { padding: 0 !important; }
        }
        .print-document { max-width: 190mm; margin: 0 auto; padding: 24px 16px; }
        .print-document table { width: 100%; border-collapse: collapse; }
        .print-document th, .print-document td {
          border-bottom: 1px solid #d9ddd2; padding: 6px 8px; text-align: left;
          font-size: 12px; vertical-align: top;
        }
        .print-document th { font-weight: 600; }
        .print-document .num { text-align: right; font-variant-numeric: tabular-nums; }
      `}</style>

      <header className="flex items-start justify-between gap-6 mb-6">
        <div>
          <div className="text-[11px] uppercase tracking-wide" style={{ color: '#5a6152' }}>
            QOIM · Самари Кӯҳсор
          </div>
          <h1 className="text-[22px] mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
            {title}
          </h1>
          {subtitle && (
            <div className="text-[13px] mt-0.5" style={{ color: '#5a6152' }}>
              {subtitle}
            </div>
          )}
        </div>
        <button
          type="button"
          className="btn btn-secondary print-hide"
          onClick={() => window.print()}
          data-testid="print-button"
        >
          Печать
        </button>
      </header>

      {children}

      <footer className="mt-8 pt-3 text-[11px]" style={{ borderTop: '1px solid #d9ddd2', color: '#5a6152' }}>
        Документ сформирован системой Самари Кӯҳсор ·{' '}
        {new Date().toLocaleString('ru-RU', { timeZone: 'Asia/Dushanbe' })}
      </footer>
    </div>
  );
}

/** A two-column definition block, the shape every one of these documents needs. */
export function PrintFields({ fields }: { fields: Array<{ label: string; value: ReactNode }> }) {
  return (
    <dl className="grid gap-x-8 gap-y-2 mb-6" style={{ gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' }}>
      {fields.map((f) => (
        <div key={f.label} className="flex justify-between gap-3 text-[13px]">
          <dt style={{ color: '#5a6152' }}>{f.label}</dt>
          <dd className="text-right">{f.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** The signature block a printed document needs to be worth printing. */
export function PrintSignatures({ roles }: { roles: string[] }) {
  return (
    <div className="grid gap-8 mt-10" style={{ gridTemplateColumns: `repeat(${roles.length}, minmax(0, 1fr))` }}>
      {roles.map((role) => (
        <div key={role}>
          <div style={{ borderBottom: '1px solid #3a4034', height: 28 }} />
          <div className="text-[11px] mt-1" style={{ color: '#5a6152' }}>
            {role}
          </div>
        </div>
      ))}
    </div>
  );
}
