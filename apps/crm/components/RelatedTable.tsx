'use client';

import type { ReactNode } from 'react';

/**
 * The "related records" band of the shared detail pattern
 * (docs/05-MODULES.md §2): batches, movements, tests, lines.
 *
 * Deliberately not `ListView`. That component owns search, KPIs, pagination and
 * empty/error states because it IS the module's screen; a related band is a
 * plain table inside somebody else's screen, and reusing ListView here would
 * drag a search box and a pager into every detail view.
 */

export interface RelatedColumn<Row> {
  key: string;
  header: string;
  numeric?: boolean;
  render: (row: Row) => ReactNode;
}

export function RelatedTable<Row>({
  title,
  columns,
  rows,
  rowKey,
  emptyLabel,
  action,
}: {
  title: string;
  columns: RelatedColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  emptyLabel: string;
  /** A create button, when the viewer may add to this collection. */
  action?: ReactNode;
}) {
  return (
    <section className="card" data-testid="related-table">
      <div
        className="flex items-center justify-between gap-3 p-4 border-b"
        style={{ borderColor: 'var(--color-divider)' }}
      >
        <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
          {title}
        </h2>
        {action}
      </div>

      {rows.length === 0 ? (
        <p className="muted text-[13px] p-4" data-testid="related-empty">
          {emptyLabel}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th
                    key={c.key}
                    className={c.numeric ? 'text-right' : undefined}
                    scope="col"
                  >
                    {c.header}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={rowKey(row)} data-testid="related-row">
                  {columns.map((c) => (
                    <td key={c.key} className={c.numeric ? 'text-right tabular-nums' : undefined}>
                      {c.render(row)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
