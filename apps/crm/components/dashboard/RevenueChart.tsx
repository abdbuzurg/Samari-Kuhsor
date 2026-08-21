'use client';

import type { Dashboard } from '@samari/types';

/**
 * Выручка и заказы — the dual bar chart from the approved prototype.
 *
 * Two bars per period, side by side: revenue in the brand accent, order count in
 * a tinted second accent. That pairing is the point of the panel — revenue alone
 * cannot distinguish a good month from one big order, and the two bars diverging
 * is the thing a director is looking for.
 *
 * An earlier pass drew a single revenue polyline instead. It was a different
 * chart answering a different question, and it dropped the order count the
 * prototype puts beside it.
 *
 * Bar heights are `value / max * 150 + 8`, exactly as the prototype computes
 * them: the +8 floor keeps a zero-value period visible as a stub rather than
 * vanishing, which matters on a system that starts empty.
 */
export function RevenueChart({
  points,
  revenueLabel,
  ordersLabel,
}: {
  points: Dashboard['revenue'];
  revenueLabel: string;
  ordersLabel: string;
}) {
  const revenues = points.map((p) => Number(p.revenue));
  const orders = points.map((p) => p.order_count);
  // Guarded at 1 so an all-zero series divides cleanly and every bar renders as
  // the 8px stub rather than NaN.
  const maxRevenue = Math.max(...revenues, 1);
  const maxOrders = Math.max(...orders, 1);

  return (
    <div
      className="flex items-stretch gap-2"
      style={{ height: 196 }}
      role="img"
      aria-label={`${revenueLabel}, ${ordersLabel}`}
      data-testid="revenue-chart"
    >
      {points.map((p, i) => (
        <div
          key={p.day}
          className="flex-1 flex flex-col items-center justify-end gap-[7px]"
          data-testid="chart-column"
        >
          <div className="flex items-end gap-1" style={{ height: 158 }}>
            <div
              data-testid="bar-revenue"
              style={{
                width: 13,
                height: Math.round((revenues[i] / maxRevenue) * 150) + 8,
                borderRadius: 'var(--radius-sm) var(--radius-sm) 0 0',
                background: 'var(--color-accent)',
                transition: 'height .4s cubic-bezier(.2,.7,.3,1)',
              }}
            />
            <div
              data-testid="bar-orders"
              style={{
                width: 13,
                height: Math.round((orders[i] / maxOrders) * 150) + 8,
                borderRadius: 'var(--radius-sm) var(--radius-sm) 0 0',
                background:
                  'color-mix(in srgb, var(--color-accent-2) 62%, var(--color-surface))',
                transition: 'height .4s cubic-bezier(.2,.7,.3,1)',
              }}
            />
          </div>
          <div className="muted" style={{ fontSize: 10.5 }}>
            {shortDay(p.day)}
          </div>
        </div>
      ))}
    </div>
  );
}

/** The chart's own legend, so the two bars are identifiable without a tooltip. */
export function ChartLegend({
  revenueLabel,
  ordersLabel,
}: {
  revenueLabel: string;
  ordersLabel: string;
}) {
  return (
    <div className="muted flex items-center gap-3 text-[11.5px]">
      <span className="inline-flex items-center gap-1.5">
        <span
          aria-hidden
          className="inline-block w-2.5 h-2.5 rounded-sm"
          style={{ background: 'var(--color-accent)' }}
        />
        {revenueLabel}
      </span>
      <span className="inline-flex items-center gap-1.5">
        <span
          aria-hidden
          className="inline-block w-2.5 h-2.5 rounded-sm"
          style={{
            background: 'color-mix(in srgb, var(--color-accent-2) 62%, var(--color-surface))',
          }}
        />
        {ordersLabel}
      </span>
    </div>
  );
}

/**
 * `2026-08-17` → `17.08`.
 *
 * The axis has room for a handful of characters per column and the year is the
 * same on every one of them, so it carries no information here.
 */
function shortDay(day: string): string {
  const parts = day.split('-');
  return parts.length === 3 ? `${parts[2]}.${parts[1]}` : day;
}
