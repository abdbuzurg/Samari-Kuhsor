'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { usePurchaseOrders } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { PurchaseOrderRow } from '@samari/types';

/** Закупки — list view. Columns from docs/05-MODULES.md §8. */
export default function ProcurementPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const orders = usePurchaseOrders({ q: search, page });
  const rows = orders.data?.data ?? [];
  const meta = orders.data?.meta;

  const kpis: KPI[] = [
    { label: 'Заказов поставщикам', value: meta ? String(meta.total) : '—' },
    {
      label: 'На согласовании',
      value: String(rows.filter((r) => r.status.key === 'approval').length),
      // Amber, because an order waiting for approval is blocking someone.
      delta: rows.some((r) => r.status.key === 'approval')
        ? { text: 'требует решения', direction: 'warn' }
        : undefined,
    },
    { label: 'В пути', value: String(rows.filter((r) => r.status.key === 'in_transit').length) },
    { label: 'Сумма, с.', value: sumTotals(rows) },
  ];

  const columns: Column<PurchaseOrderRow>[] = [
    { key: 'po_no', header: 'Заказ', render: (r) => <span className="tabular-nums">{r.po_no}</span> },
    { key: 'supplier', header: 'Поставщик', render: (r) => r.supplier_name },
    { key: 'expected', header: 'Ожидается', render: (r) => orTBC(r.expected_at) },
    {
      key: 'total',
      header: 'Сумма',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.total} с.</span>,
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<PurchaseOrderRow>
        kicker={t('group.ops')}
        title={t('mod.procurement')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/procurement/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру заказа и поставщику"
        createLabel={t('create')}
        isLoading={orders.isLoading}
        error={orders.isError ? { message: 'Не удалось загрузить заказы поставщикам' } : null}
        emptyTitle="Заказов поставщикам нет"
        emptyBody="Создайте первый заказ, чтобы вести закупки сырья и упаковки."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

function sumTotals(rows: PurchaseOrderRow[]): string {
  if (rows.length === 0) return '—';
  return rows.reduce((sum, r) => sum + Number(r.total), 0).toFixed(2);
}
