'use client';

import Link from 'next/link';
import { ChevronRight } from 'lucide-react';
import type { ReactNode } from 'react';

/**
 * The shared record detail pattern, per docs/05-MODULES.md §2:
 *
 *   Breadcrumb: Модуль / Запись
 *   Header:     title · key identifier · status tag · [Редактировать] [actions]
 *   Field groups: 2-column definition list, grouped by theme
 *   Related records: tabbed or stacked tables
 *   Activity:   audit_log entries for this record, newest first
 *   Footer:     created/updated by and when, version
 *
 * `HANDOFF-CRM-CONTEXT.md:346` calls the detail view "the single largest piece of
 * remaining design work" — it exists in no prototype. Building it once here is
 * why every later module gets one for free.
 *
 * Two rules from §2 that this component's shape enforces rather than documents:
 *   - Action buttons appear only when the caller passes them, so a module cannot
 *     accidentally render an action the user may not take. Hiding is cosmetic;
 *     the server still refuses.
 *   - State transitions are ACTIONS, never a status dropdown in the edit form.
 *     There is deliberately no way to pass a status field here.
 */

export interface Field {
  label: string;
  value: ReactNode;
  /** Span both columns — for long text like a description. */
  wide?: boolean;
}

export interface FieldGroup {
  title: string;
  fields: Field[];
}

export interface DetailViewProps {
  moduleLabel: string;
  moduleHref: string;
  /** The record's own label in the breadcrumb. */
  recordLabel: string;

  title: string;
  /** The key identifier shown beside the title — a SKU, a batch number, a PO number. */
  identifier?: string;
  status?: ReactNode;
  actions?: ReactNode;

  groups: FieldGroup[];
  /** Related records: batches, movements, tests, lines. */
  related?: ReactNode;
  /** Activity panel — audit_log for this record. */
  activity?: ReactNode;

  footer?: {
    createdAt: string;
    updatedAt: string;
    version: number;
  };
}

export function DetailView({
  moduleLabel,
  moduleHref,
  recordLabel,
  title,
  identifier,
  status,
  actions,
  groups,
  related,
  activity,
  footer,
}: DetailViewProps) {
  return (
    <div data-testid="detail-view">
      <nav className="flex items-center gap-1.5 text-[12px] muted mb-3" aria-label="Хлебные крошки">
        <Link href={moduleHref} className="hover:underline">
          {moduleLabel}
        </Link>
        <ChevronRight size={13} aria-hidden />
        <span aria-current="page">{recordLabel}</span>
      </nav>

      <div className="flex items-start gap-4 mb-5">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-3 flex-wrap">
            <h1
              className="text-[27px] leading-[1.05]"
              style={{ fontFamily: 'var(--font-heading)' }}
            >
              {title}
            </h1>
            {status}
          </div>
          {identifier && <div className="text-[13px] muted mt-1 tabular-nums">{identifier}</div>}
        </div>
        {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
      </div>

      {/* Content beside activity at desktop; stacked below lg, where a 1fr
          activity column would be ~120px wide and unreadable. */}
      <div className="grid gap-4 grid-cols-1 lg:[grid-template-columns:minmax(0,2fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-4 min-w-0">
          {groups.map((group) => (
            <section key={group.title} className="card p-5">
              <h2
                className="text-[15px] mb-3"
                style={{ fontFamily: 'var(--font-heading)' }}
              >
                {group.title}
              </h2>
              <dl className="grid gap-x-6 gap-y-3 grid-cols-1 sm:grid-cols-2">
                {group.fields.map((f) => (
                  <div key={f.label} style={f.wide ? { gridColumn: '1 / -1' } : undefined}>
                    <dt className="text-[12px] muted">{f.label}</dt>
                    <dd className="text-[13px] mt-0.5">{f.value}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
          {related}
        </div>

        <div className="flex flex-col gap-4 min-w-0">
          {activity}
          {footer && (
            <section className="card p-4 text-[12px] muted">
              <div>Создано: {footer.createdAt}</div>
              <div>Изменено: {footer.updatedAt}</div>
              <div className="tabular-nums">Версия: {footer.version}</div>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}
