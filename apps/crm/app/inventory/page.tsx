'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useStock } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { StockBalanceRow } from '@samari/types';

/**
 * Склад и запасы — list view. Columns from docs/05-MODULES.md §5.
 *
 * Every quantity on this page is a SUM the server computed at read time. There
 * is no stored balance behind any of them (CLAUDE.md §4.2), which is why a row
 * showing 480 can always be explained by opening its ledger.
 */
export default function InventoryPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const stock = useStock({ q: search, page });
  const rows = stock.data?.data ?? [];
  const meta = stock.data?.meta;

  // KPIs from docs/05-MODULES.md §5. "Позиций ниже минимума" is the one that
  // matters operationally, so it is counted from the rows' own status rather
  // than a second query that could disagree with what is on screen.
  const kpis: KPI[] = [
    { label: 'Позиций на складе', value: meta ? String(meta.total) : '—' },
    {
      label: 'Ниже минимума',
      value: String(rows.filter((r) => r.status.key === 'below_minimum').length),
    },
    {
      label: 'Заканчивается',
      value: String(rows.filter((r) => r.status.key === 'low').length),
    },
    {
      label: 'В карантине',
      value: String(rows.filter((r) => r.location_zone === 'quarantine').length),
    },
  ];

  const columns: Column<StockBalanceRow>[] = [
    { key: 'sku', header: 'Артикул', render: (r) => <span className="tabular-nums">{r.sku}</span> },
    { key: 'name', header: 'Наименование', render: (r) => r.item_name },
    { key: 'batch', header: 'Партия', render: (r) => orTBC(r.batch_no) },
    { key: 'location', header: 'Локация', render: (r) => r.location_code },
    {
      key: 'on_hand',
      header: 'Остаток',
      numeric: true,
      // A string, straight through. Rendering it via Number would reintroduce
      // the float that the string wire format exists to avoid.
      render: (r) => (
        <span className="tabular-nums">
          {r.on_hand} {r.base_uom}
        </span>
      ),
    },
    {
      key: 'min_qty',
      header: 'Минимум',
      numeric: true,
      render: (r) => <span className="tabular-nums">{orTBC(r.min_qty)}</span>,
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<StockBalanceRow>
        kicker={t('group.ops')}
        title={t('mod.inventory')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => `${r.item_id}:${r.batch_id ?? '-'}:${r.location_id}`}
        rowHref={(r) =>
          `/inventory/ledger?item_id=${r.item_id}&location_id=${r.location_id}` +
          (r.batch_id ? `&batch_id=${r.batch_id}` : '')
        }
        exportKey="stock"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по артикулу, наименованию и партии"
        createLabel={t('create')}
        isLoading={stock.isLoading}
        error={stock.isError ? { message: 'Не удалось загрузить остатки' } : null}
        emptyTitle="Складских остатков нет"
        emptyBody="Позиции появятся после первой приёмки или выпуска продукции."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
