'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useSalesOrders } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { SalesOrderRow } from '@samari/types';

/** Продажи — list view. Columns from docs/05-MODULES.md §9. */
export default function SalesPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const orders = useSalesOrders({ q: search, page });
  const rows = orders.data?.data ?? [];
  const meta = orders.data?.meta;

  const kpis: KPI[] = [
    { label: 'Заказов клиентов', value: meta ? String(meta.total) : '—' },
    { label: 'Черновики', value: String(rows.filter((r) => r.status.key === 'draft').length) },
    { label: 'Отгружено', value: String(rows.filter((r) => r.status.key === 'shipped').length) },
    { label: 'Сумма, с.', value: sumTotals(rows) },
  ];

  const columns: Column<SalesOrderRow>[] = [
    { key: 'so_no', header: 'Заказ', render: (r) => <span className="tabular-nums">{r.so_no}</span> },
    { key: 'customer', header: 'Клиент', render: (r) => r.customer_name },
    { key: 'ordered', header: 'Дата заказа', render: (r) => orTBC(r.ordered_on) },
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
      <ListView<SalesOrderRow>
        kicker={t('group.sales')}
        title={t('mod.crm')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/crm/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру заказа и клиенту"
        createLabel={t('create')}
        isLoading={orders.isLoading}
        error={orders.isError ? { message: 'Не удалось загрузить заказы клиентов' } : null}
        emptyTitle="Заказов клиентов нет"
        emptyBody="Создайте первый заказ или конвертируйте обращение с сайта."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

function sumTotals(rows: SalesOrderRow[]): string {
  if (rows.length === 0) return '—';
  return rows.reduce((sum, r) => sum + Number(r.total), 0).toFixed(2);
}
