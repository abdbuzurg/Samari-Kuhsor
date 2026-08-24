'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useManufacturingOrders } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { ManufacturingOrderRow } from '@samari/types';

/** Производство — list view. Columns from docs/05-MODULES.md §6. */
export default function ProductionPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'production', 'manage');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const orders = useManufacturingOrders({ q: search, page });
  const rows = orders.data?.data ?? [];
  const meta = orders.data?.meta;

  const kpis: KPI[] = [
    { label: 'Заказов на производство', value: meta ? String(meta.total) : '—' },
    {
      label: 'В работе',
      value: String(rows.filter((r) => r.status.key === 'in_progress').length),
    },
    {
      label: 'Завершено',
      value: String(rows.filter((r) => r.status.key === 'done').length),
    },
    { label: 'Выпущено, ед.', value: totalGood(rows) },
  ];

  const columns: Column<ManufacturingOrderRow>[] = [
    { key: 'mo_no', header: 'Заказ', render: (r) => <span className="tabular-nums">{r.mo_no}</span> },
    { key: 'name', header: 'Продукция', render: (r) => r.item_name },
    { key: 'batch', header: 'Партия', render: (r) => orTBC(r.batch_no) },
    { key: 'line', header: 'Линия', render: (r) => orTBC(r.line) },
    {
      key: 'planned',
      header: 'План',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.planned_qty}</span>,
    },
    {
      key: 'good',
      header: 'Выпущено',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.good_qty}</span>,
    },
    {
      key: 'scrap',
      header: 'Брак',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.scrap_qty}</span>,
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<ManufacturingOrderRow>
        kicker={t('group.ops')}
        title={t('mod.production')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/production/${r.id}`}
        exportKey="production"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру заказа и артикулу"
        onCreate={mayManage ? () => router.push('/production/new') : undefined}
        createLabel={t('create')}
        isLoading={orders.isLoading}
        error={orders.isError ? { message: 'Не удалось загрузить заказы на производство' } : null}
        emptyTitle="Заказов на производство нет"
        emptyBody="Запланируйте первый заказ, чтобы начать учёт выпуска."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

/** Sum of good output on this page, as a string.
 *
 *  Summed through Number because it is a display total over at most one page,
 *  not a stored figure — nothing is written back from it. The authoritative
 *  per-order numbers stay strings all the way from Postgres. */
function totalGood(rows: ManufacturingOrderRow[]): string {
  if (rows.length === 0) return '—';
  return String(rows.reduce((sum, r) => sum + Number(r.good_qty), 0));
}
