'use client';

import type { AnalyticsReport } from '@samari/types';

/**
 * «Что смотрят на сайте» и «Популярные ссылки».
 *
 * Ranked by VISITS, not events. Ten views in one session count once, which is
 * the whole reason a visit identifier is carried at all — without it one
 * enthusiastic distributor outranks thirty real people and the owner reads noise
 * as demand (docs/01-DECISIONS.md D12).
 *
 * Bars are scaled against the leader, like PipelinePanel, so the shape of the
 * ranking is readable at a glance rather than needing the numbers read.
 */
export function SiteAnalyticsPanel({ report }: { report: AnalyticsReport }) {
  const products = report.products ?? [];
  const links = report.links ?? [];

  if (products.length === 0 && links.length === 0) {
    return (
      <p className="muted text-[13px]" data-testid="analytics-empty">
        Пока нет данных. Статистика собирается только с посетителей, которые приняли
        баннер на сайте.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-5" data-testid="site-analytics">
      <div className="flex gap-6 text-[13px]">
        <span>
          <span className="muted">Визитов: </span>
          <strong className="tabular-nums">{report.visits}</strong>
        </span>
        <span>
          <span className="muted">Смотрели товары: </span>
          <strong className="tabular-nums">{report.product_visits}</strong>
        </span>
      </div>

      {products.length > 0 && (
        <Ranking
          title="Что смотрят на сайте"
          testid="analytics-products"
          rows={products.map((p) => ({
            key: p.sku,
            label: p.name,
            hint: p.sku,
            value: p.visits,
          }))}
        />
      )}

      {links.length > 0 && (
        <Ranking
          title="Популярные ссылки"
          testid="analytics-links"
          rows={links.map((l) => ({
            key: l.target,
            label: l.target,
            hint: l.category === 'outbound' ? 'внешняя' : 'кнопка',
            value: l.visits,
          }))}
        />
      )}
    </div>
  );
}

function Ranking({
  title,
  testid,
  rows,
}: {
  title: string;
  testid: string;
  rows: Array<{ key: string; label: string; hint: string; value: number }>;
}) {
  // Scaled against the leader, not the total: this is a ranking, not a share.
  const top = Math.max(...rows.map((r) => r.value), 1);

  return (
    <section className="flex flex-col gap-2" data-testid={testid}>
      <h3 className="text-[12px] muted" style={{ letterSpacing: '.06em' }}>
        {title}
      </h3>
      {rows.map((row) => (
        <div
          key={row.key}
          className="grid items-center gap-2 text-[12.5px]"
          style={{ gridTemplateColumns: 'minmax(0,148px) 1fr 34px' }}
          data-testid="analytics-row"
        >
          <span className="truncate" title={`${row.label} · ${row.hint}`}>
            {row.label}
          </span>
          <div
            className="rounded-full overflow-hidden"
            style={{
              height: 10,
              background: 'color-mix(in srgb, var(--color-text) 9%, transparent)',
            }}
          >
            <div
              className="h-full rounded-full"
              style={{
                width: `${Math.max(4, Math.round((row.value / top) * 100))}%`,
                background: 'var(--color-accent)',
                transition: 'width .5s',
              }}
            />
          </div>
          <span className="text-right tabular-nums font-semibold">{row.value}</span>
        </div>
      ))}
    </section>
  );
}
