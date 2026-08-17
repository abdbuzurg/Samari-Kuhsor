'use client';

import Link from 'next/link';
import { Search, Plus, Download, SlidersHorizontal } from 'lucide-react';
import type { ReactNode } from 'react';

/**
 * The module list view, reproducing `moduleView()` in the approved prototype
 * (design/Samari-Kuhsor-Green-CRM.html:788).
 *
 * The prototype renders all thirteen modules from one function driven by a
 * `{kpis, cols, rows}` config — the approved design is already a generic engine,
 * which is why this is built as one component rather than thirteen.
 *
 * Structure, in the prototype's order:
 *   page header (kicker + title + actions) → KPI strip → card { toolbar, table }
 *
 * T15 extracts this into the shared engine once Товары has proven it. Nothing
 * here is items-specific.
 */

export interface KPI {
  label: string;
  value: string;
  /** Optional delta line under the value. */
  delta?: { text: string; direction: 'up' | 'down' | 'warn' | 'danger' };
}

export interface Column<Row> {
  key: string;
  header: string;
  /** Right-align numeric columns; the prototype uses tabular numerals for these. */
  numeric?: boolean;
  render: (row: Row) => ReactNode;
}

export interface ListViewProps<Row> {
  kicker: string;
  title: string;
  kpis?: KPI[];
  columns: Column<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  /** Where a row click goes. Omitted rows are not clickable. */
  rowHref?: (row: Row) => string;

  search: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;

  /** Rendered only when the user may create — hiding is cosmetic (§04-RBAC:120). */
  onCreate?: () => void;
  createLabel: string;

  isLoading: boolean;
  error?: { message: string } | null;
  /** Shown when the collection is genuinely empty rather than filtered to nothing. */
  emptyTitle: string;
  emptyBody: string;
  /** Shown when a search or filter excluded everything. */
  noMatchLabel: string;

  meta?: { page: number; total: number; total_pages: number };
  onPageChange?: (page: number) => void;
}

export function ListView<Row>({
  kicker,
  title,
  kpis = [],
  columns,
  rows,
  rowKey,
  rowHref,
  search,
  onSearchChange,
  searchPlaceholder,
  onCreate,
  createLabel,
  isLoading,
  error,
  emptyTitle,
  emptyBody,
  noMatchLabel,
  meta,
  onPageChange,
}: ListViewProps<Row>) {
  const filtered = search.trim().length > 0;

  return (
    <div data-testid="list-view">
      {/* Page header — sk-pagehd in the prototype */}
      <div className="flex items-start gap-4 mb-5">
        <div className="flex-1 min-w-0">
          <div className="text-[11px] uppercase tracking-[0.18em] muted">{kicker}</div>
          {/* flex-1 min-w-0 above so a long title never wraps behind the actions
              (HANDOFF-CRM-CONTEXT.md:165). */}
          <h1 className="text-[27px] leading-[1.05] mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
            {title}
          </h1>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button type="button" className="btn btn-secondary" disabled>
            <Download size={15} aria-hidden />
            Экспорт
          </button>
          {onCreate && (
            <button
              type="button"
              className="btn"
              style={{ background: 'var(--color-accent)', color: 'var(--color-bg)' }}
              onClick={onCreate}
            >
              <Plus size={15} aria-hidden />
              {createLabel}
            </button>
          )}
        </div>
      </div>

      {kpis.length > 0 && (
        <div
          className="grid gap-3 mb-4"
          style={{ gridTemplateColumns: `repeat(${kpis.length}, minmax(0, 1fr))` }}
          data-testid="kpi-strip"
        >
          {kpis.map((k) => (
            <div key={k.label} className="card p-4">
              <div className="text-[12px] muted">{k.label}</div>
              <div
                className="text-[24px] leading-none mt-1.5"
                style={{ fontFamily: 'var(--font-heading)', fontFeatureSettings: "'tnum'" }}
              >
                {k.value}
              </div>
              <div className="text-[12px] mt-1.5 muted">{k.delta?.text ?? ' '}</div>
            </div>
          ))}
        </div>
      )}

      <div className="card">
        {/* Toolbar — sk-toolbar */}
        <div className="flex items-center gap-2 p-3 border-b" style={{ borderColor: 'var(--color-divider)' }}>
          <div className="relative flex-1 max-w-[380px]">
            {/* 16x16 at left:12px, input padded to 38px — the prototype's
                search-icon invariant (HANDOFF-CRM-CONTEXT.md:166). */}
            <Search
              size={16}
              className="absolute left-3 top-1/2 -translate-y-1/2 muted pointer-events-none"
              aria-hidden
            />
            <input
              className="input w-full"
              style={{ paddingLeft: 38 }}
              type="search"
              value={search}
              placeholder={searchPlaceholder}
              aria-label={searchPlaceholder}
              onChange={(e) => onSearchChange(e.target.value)}
            />
          </div>
          <button type="button" className="btn btn-secondary" disabled>
            <SlidersHorizontal size={15} aria-hidden />
            Фильтры
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="table w-full">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.key} className={c.numeric ? 'text-right' : undefined}>
                    {c.header}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {/* The four states CLAUDE.md §7 requires, in one place so every
                  module gets them without re-implementing. */}
              {isLoading && (
                <tr>
                  <td colSpan={columns.length} className="muted text-center py-10" role="status">
                    Загрузка…
                  </td>
                </tr>
              )}

              {!isLoading && error && (
                <tr>
                  <td colSpan={columns.length} className="text-center py-10" role="alert">
                    {error.message}
                  </td>
                </tr>
              )}

              {!isLoading && !error && rows.length === 0 && (
                <tr>
                  <td colSpan={columns.length} className="text-center py-12">
                    {/* An empty collection and a filtered-to-nothing one are
                        different situations and get different text: "create your
                        first product" is unhelpful when the answer is "clear the
                        search box". */}
                    {filtered ? (
                      <span className="muted">
                        {noMatchLabel} «{search}»
                      </span>
                    ) : (
                      <>
                        <div className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
                          {emptyTitle}
                        </div>
                        <div className="muted text-[13px] mt-1">{emptyBody}</div>
                      </>
                    )}
                  </td>
                </tr>
              )}

              {!isLoading &&
                !error &&
                rows.map((row) => {
                  const cells = columns.map((c) => (
                    <td key={c.key} className={c.numeric ? 'text-right tabular-nums' : undefined}>
                      {c.render(row)}
                    </td>
                  ));
                  const href = rowHref?.(row);
                  return (
                    <tr key={rowKey(row)} data-testid="list-row">
                      {href
                        ? cells.map((cell, i) =>
                            i === 0 ? (
                              <td key={columns[i].key}>
                                <Link href={href} className="hover:underline">
                                  {columns[i].render(row)}
                                </Link>
                              </td>
                            ) : (
                              cell
                            ),
                          )
                        : cells}
                    </tr>
                  );
                })}
            </tbody>
          </table>
        </div>

        {meta && meta.total_pages > 1 && (
          <div
            className="flex items-center justify-between p-3 border-t text-[13px]"
            style={{ borderColor: 'var(--color-divider)' }}
          >
            <span className="muted">
              Страница {meta.page} из {meta.total_pages} · всего {meta.total}
            </span>
            <div className="flex gap-2">
              <button
                type="button"
                className="btn btn-secondary"
                disabled={meta.page <= 1}
                onClick={() => onPageChange?.(meta.page - 1)}
              >
                Назад
              </button>
              <button
                type="button"
                className="btn btn-secondary"
                disabled={meta.page >= meta.total_pages}
                onClick={() => onPageChange?.(meta.page + 1)}
              >
                Вперёд
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
