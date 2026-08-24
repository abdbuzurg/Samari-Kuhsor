'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useAssets } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { Asset } from '@samari/types';

/** Оборудование и ТО — list view. Columns from docs/05-MODULES.md §13. */
export default function EquipmentPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'equipment', 'manage');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const assets = useAssets({ q: search, page });
  const rows = assets.data?.data ?? [];
  const meta = assets.data?.meta;

  const broken = rows.filter((r) => r.status.key === 'broken').length;
  const kpis: KPI[] = [
    { label: 'Единиц оборудования', value: meta ? String(meta.total) : '—' },
    { label: 'В работе', value: String(rows.filter((r) => r.status.key === 'running').length) },
    {
      label: 'Требуют ТО',
      value: String(rows.filter((r) => r.status.key === 'maintenance_due').length),
    },
    {
      label: 'Неисправны',
      value: String(broken),
      // Broken equipment stops the line, which is why it is called out rather
      // than shown as one status among four.
      delta: broken > 0 ? { text: 'линия остановлена', direction: 'danger' } : undefined,
    },
  ];

  const columns: Column<Asset>[] = [
    {
      key: 'asset_no',
      header: 'Инв. №',
      render: (r) => <span className="tabular-nums">{r.asset_no}</span>,
    },
    { key: 'name', header: 'Наименование', render: (r) => r.name },
    { key: 'line', header: 'Линия', render: (r) => orTBC(r.line) },
    { key: 'commissioned', header: 'Ввод в эксплуатацию', render: (r) => orTBC(r.commissioned_on) },
    { key: 'next_due', header: 'Следующее ТО', render: (r) => orTBC(r.next_due_on) },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<Asset>
        kicker={t('group.admin')}
        title={t('mod.equipment')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/equipment/${r.id}`}
        exportKey="equipment"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по инвентарному номеру, названию и линии"
        onCreate={mayManage ? () => router.push('/equipment/new') : undefined}
        createLabel={t('create')}
        isLoading={assets.isLoading}
        error={assets.isError ? { message: 'Не удалось загрузить список оборудования' } : null}
        emptyTitle="Оборудование не заведено"
        emptyBody="Добавьте первую единицу, чтобы планировать техобслуживание."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
