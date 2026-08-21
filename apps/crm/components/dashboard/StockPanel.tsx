'use client';

import { StatusTag } from '@/components/StatusTag';
import type { Dashboard } from '@samari/types';

/**
 * Запасы: остатки и сроки.
 *
 * Five rows out of hundreds of positions, so the ORDER is the selection: the
 * server sorts below-minimum first, then soonest-to-expire. A panel this small
 * that showed the alphabetically-first five positions would be decoration.
 *
 * The subtitle carries whichever identifier is meaningful for the row — a batch
 * number for finished goods, a bin location for raw materials. Showing both
 * would not fit and showing neither leaves a warehouse operator unable to act.
 */
export function StockPanel({ rows }: { rows: Dashboard['stock_rows'] }) {
  if (rows.length === 0) {
    return (
      <p className="muted text-[13px]" data-testid="stock-panel-empty">
        Складских остатков нет.
      </p>
    );
  }

  return (
    <ul className="flex flex-col" data-testid="stock-panel">
      {rows.map((row) => (
        <li
          key={`${row.sku}-${row.detail}`}
          className="flex items-center gap-3 py-2.5 border-b last:border-b-0"
          style={{ borderColor: 'var(--color-divider)' }}
          data-testid="stock-panel-row"
        >
          <span className="flex-1 min-w-0">
            <span className="block text-[13px] truncate">{row.name}</span>
            <span className="block text-[11.5px] muted truncate">
              {/* Quantity and the identifier, in the prototype's order: how much
                  first, then which one. */}
              {row.on_hand} {row.uom} · {row.detail}
            </span>
          </span>
          <StatusTag status={row.status} />
        </li>
      ))}
    </ul>
  );
}
