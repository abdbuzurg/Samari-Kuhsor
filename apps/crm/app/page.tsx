'use client';

import { useQuery } from '@tanstack/react-query';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { StatusTag } from '@/components/StatusTag';
import { ApiError } from '@/lib/session';
import type { Dashboard, DashboardEvent } from '@samari/types';

/**
 * Панель управления — docs/05-MODULES.md §2. Built last, because it summarises
 * every module beneath it.
 *
 * Two things this page deliberately does NOT do.
 *
 * It does not carry the prototype's sample figures. 05-MODULES.md:70 is explicit,
 * and on opening day these panels are genuinely empty: showing 2 480 000 c. of
 * revenue for a factory that has produced nothing would be a lie the client would
 * reasonably act on. Zero is rendered as zero.
 *
 * And it does not render a panel the viewer may not see. The server sends null
 * for a module the user cannot read — null and 0 look identical on screen and
 * mean completely different things, so the panel is omitted entirely rather than
 * showing a figure the user is not entitled to.
 */
export default function DashboardPage() {
  const t = useTranslations();
  const [period, setPeriod] = useState('month');

  const dashboard = useQuery<Dashboard>({
    queryKey: ['dashboard', period],
    queryFn: async () => {
      const res = await fetch(`/api/dashboard?period=${period}`, {
        headers: { 'Content-Type': 'application/json' },
      });
      const body = await res.json();
      if (!res.ok) {
        throw new ApiError(
          res.status,
          body?.error?.code ?? 'internal_error',
          body?.error?.message ?? '',
        );
      }
      return body.data as Dashboard;
    },
    placeholderData: (previous) => previous,
  });

  const d = dashboard.data;

  return (
    <AppShell>
      <div className="mb-5 flex items-end justify-between gap-4">
        <div>
          <div className="text-[11px] uppercase tracking-[0.18em] muted">
            {t('group.overview')}
          </div>
          <h1
            className="text-[27px] leading-tight mt-1"
            style={{ fontFamily: 'var(--font-heading)' }}
          >
            {t('mod.dashboard')}
          </h1>
        </div>
        <div className="flex gap-1" role="group" aria-label="Период">
          {(['day', 'week', 'month', 'quarter'] as const).map((p) => (
            <button
              key={p}
              type="button"
              className={`btn ${p === period ? '' : 'btn-secondary'}`}
              aria-pressed={p === period}
              onClick={() => setPeriod(p)}
            >
              {t(`period.${p}`)}
            </button>
          ))}
        </div>
      </div>

      {dashboard.isLoading && (
        <div className="card p-6 muted text-[13px]" role="status" data-testid="dashboard-loading">
          Загрузка…
        </div>
      )}

      {dashboard.isError && (
        <div className="card p-6 text-[13px]" role="alert" data-testid="dashboard-error">
          Не удалось загрузить панель управления.
        </div>
      )}

      {d && (
        <>
          <div className="grid gap-3 mb-4 md:grid-cols-3 lg:grid-cols-6" data-testid="kpi-strip">
            {/* Each KPI is rendered only when its panel came back. A missing panel
                means "you may not read this module", which is not the same as 0. */}
            {d.sales && (
              <Kpi label={t('kpi.rev')} value={`${d.sales.revenue} с.`} sub={t('kpi.revsub')} />
            )}
            {d.sales && <Kpi label={t('kpi.orders')} value={String(d.sales.open_orders)} />}
            {d.stock && <Kpi label={t('kpi.stockv')} value={`${d.stock.value} с.`} />}
            {d.quality && (
              <Kpi
                label={t('kpi.qc')}
                value={String(d.quality.quarantined)}
                danger={d.quality.quarantined > 0}
              />
            )}
            {d.sales && (
              <Kpi
                label={t('kpi.late')}
                value={String(d.sales.overdue_purchase_orders)}
                danger={d.sales.overdue_purchase_orders > 0}
              />
            )}
            {d.stock && (
              <Kpi
                label="Ниже минимума"
                value={String(d.stock.below_minimum)}
                danger={d.stock.below_minimum > 0}
              />
            )}
          </div>

          <div className="grid gap-3 lg:grid-cols-2">
            {d.sales && (
              <Panel title={t('dash.rev')}>
                {d.revenue.every((p) => p.revenue === '0.00') ? (
                  <Empty>Продаж за выбранный период пока нет.</Empty>
                ) : (
                  <Sparkline points={d.revenue} />
                )}
              </Panel>
            )}

            {d.production && (
              <Panel title={t('dash.prod')}>
                <div className="flex items-baseline gap-6">
                  <Figure label={t('dash.plan')} value={d.production.planned_qty} />
                  <Figure label={t('dash.fact')} value={d.production.good_qty} />
                  <Figure label="Брак" value={d.production.scrap_qty} />
                </div>
                <div
                  className="mt-3 h-2 rounded-full overflow-hidden"
                  style={{ background: 'var(--color-divider)' }}
                  role="progressbar"
                  aria-valuenow={d.production.progress}
                  aria-valuemin={0}
                  aria-valuemax={100}
                >
                  <div
                    className="h-full"
                    style={{
                      width: `${d.production.progress}%`,
                      background: 'var(--level-ok)',
                    }}
                  />
                </div>
              </Panel>
            )}

            {d.sales && (
              <Panel title={t('dash.orders')}>
                {d.recent_orders.length === 0 ? (
                  <Empty>Заказов ещё не было.</Empty>
                ) : (
                  <table className="table w-full">
                    <tbody>
                      {d.recent_orders.map((o) => (
                        <tr key={o.id} data-testid="recent-order">
                          <td className="tabular-nums">{o.so_no}</td>
                          <td>{o.customer_name}</td>
                          <td className="text-right tabular-nums">{o.total} с.</td>
                          <td>
                            <StatusTag status={o.status} />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </Panel>
            )}

            <Panel title={t('dash.feed')}>
              {d.feed.length === 0 ? (
                <Empty>Событий пока нет.</Empty>
              ) : (
                <ul className="space-y-2">
                  {d.feed.map((e) => (
                    <li key={e.id} className="text-[13px]" data-testid="feed-entry">
                      <span className="muted tabular-nums">
                        {new Date(e.occurred_at).toLocaleString('ru-RU')}
                      </span>{' '}
                      {e.actor_name || 'Система'} — {eventLabel(e)}
                    </li>
                  ))}
                </ul>
              )}
            </Panel>
          </div>
        </>
      )}
    </AppShell>
  );
}

function Kpi({
  label,
  value,
  sub,
  danger,
}: {
  label: string;
  value: string;
  sub?: string;
  danger?: boolean;
}) {
  return (
    <div className="card p-4" data-testid="kpi">
      <div className="text-[12px] muted">{label}</div>
      <div
        className="text-[24px] leading-none mt-1.5"
        style={{
          fontFamily: 'var(--font-heading)',
          fontFeatureSettings: "'tnum'",
          // Green means healthy, so a count of problems is never green — it is
          // either red or it is plain (CLAUDE.md §5).
          color: danger ? 'var(--level-danger)' : undefined,
        }}
      >
        {value}
      </div>
      <div className="text-[12px] mt-1.5 muted">{sub ?? ' '}</div>
    </div>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="card p-4" data-testid="panel" aria-label={title}>
      <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
        {title}
      </h2>
      {children}
    </section>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <p className="muted text-[13px]" data-testid="panel-empty">
      {children}
    </p>
  );
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[12px] muted">{label}</div>
      <div className="text-[20px] tabular-nums" style={{ fontFamily: 'var(--font-heading)' }}>
        {value}
      </div>
    </div>
  );
}

/** A bare SVG sparkline. No charting library: one polyline is the whole
 *  requirement, and a dependency here would be 40 kB for a shape. */
function Sparkline({ points }: { points: Dashboard['revenue'] }) {
  const values = points.map((p) => Number(p.revenue));
  const max = Math.max(...values, 1);
  const step = points.length > 1 ? 100 / (points.length - 1) : 100;
  const path = values.map((v, i) => `${i * step},${30 - (v / max) * 28}`).join(' ');

  return (
    <svg viewBox="0 0 100 30" className="w-full h-24" preserveAspectRatio="none" role="img"
      aria-label="Выручка по дням" data-testid="sparkline">
      <polyline
        points={path}
        fill="none"
        stroke="var(--level-ok)"
        strokeWidth="1"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

/** Renders an audit entry.
 *
 *  `action` and `resource` are keys, not sentences — the server sends them so
 *  the frontend can render in the reader's locale (docs/07 C3). */
function eventLabel(e: DashboardEvent): string {
  const actions: Record<string, string> = {
    create: 'создал',
    update: 'изменил',
    delete: 'удалил',
    approve: 'согласовал',
    login: 'вошёл в систему',
    logout: 'вышел из системы',
  };
  const resources: Record<string, string> = {
    crm: 'заказ',
    inquiries: 'обращение',
    items: 'товар',
    inventory: 'движение по складу',
    procurement: 'закупку',
    production: 'заказ на производство',
    quality: 'партию',
    logistics: 'рейс',
    hr: 'сотрудника',
    equipment: 'оборудование',
    documents: 'документ',
    admin: 'настройки доступа',
  };
  const verb = actions[e.action] ?? e.action;
  const object = resources[e.resource];
  return object ? `${verb} ${object}` : verb;
}
