'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useShipments } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { Shipment } from '@samari/types';

/** Логистика — list view. Columns from docs/05-MODULES.md §10. */
export default function LogisticsPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const trips = useShipments({ q: search, page });
  const rows = trips.data?.data ?? [];
  const meta = trips.data?.meta;

  const kpis: KPI[] = [
    { label: 'Рейсов', value: meta ? String(meta.total) : '—' },
    { label: 'Погрузка', value: String(rows.filter((r) => r.status.key === 'loading').length) },
    { label: 'В пути', value: String(rows.filter((r) => r.status.key === 'in_transit').length) },
    { label: 'Доставлено', value: String(rows.filter((r) => r.status.key === 'delivered').length) },
  ];

  const columns: Column<Shipment>[] = [
    { key: 'trip_no', header: 'Рейс', render: (r) => <span className="tabular-nums">{r.trip_no}</span> },
    {
      key: 'route',
      header: 'Маршрут',
      // Two nulls would render as «уточняется — уточняется», which is noise. A
      // route with neither end recorded is simply not yet planned.
      render: (r) =>
        r.route_from || r.route_to ? `${orTBC(r.route_from)} → ${orTBC(r.route_to)}` : orTBC(null),
    },
    { key: 'driver', header: 'Водитель', render: (r) => orTBC(r.driver_name) },
    { key: 'vehicle', header: 'Транспорт', render: (r) => orTBC(r.vehicle_plate) },
    {
      key: 'cost',
      header: 'Стоимость',
      numeric: true,
      render: (r) => (
        <span className="tabular-nums">{r.transport_cost ? `${r.transport_cost} с.` : orTBC(null)}</span>
      ),
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<Shipment>
        kicker={t('group.ops')}
        title={t('mod.logistics')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/logistics/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру рейса"
        createLabel={t('create')}
        isLoading={trips.isLoading}
        error={trips.isError ? { message: 'Не удалось загрузить рейсы' } : null}
        emptyTitle="Рейсов нет"
        emptyBody="Создайте рейс, чтобы отгрузить подтверждённые заказы."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
