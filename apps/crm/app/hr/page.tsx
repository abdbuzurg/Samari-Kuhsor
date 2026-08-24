'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useEmployees } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { Employee } from '@samari/types';

/**
 * Персонал — list view. Columns from docs/05-MODULES.md §12.
 *
 * The server orders by contract expiry, because that is the question this
 * register exists to answer. This page does not re-sort.
 */
export default function HRPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'hr', 'manage');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const employees = useEmployees({ q: search, page });
  const rows = employees.data?.data ?? [];
  const meta = employees.data?.meta;

  const kpis: KPI[] = [
    { label: 'Сотрудников', value: meta ? String(meta.total) : '—' },
    { label: 'Работают', value: String(rows.filter((r) => r.status.key === 'active').length) },
    { label: 'В отпуске', value: String(rows.filter((r) => r.status.key === 'on_leave').length) },
    { label: 'Договоры истекают', value: String(expiringSoon(rows)) },
  ];

  const columns: Column<Employee>[] = [
    { key: 'name', header: 'ФИО', render: (r) => r.full_name },
    { key: 'position', header: 'Должность', render: (r) => orTBC(r.position_title) },
    { key: 'department', header: 'Подразделение', render: (r) => orTBC(r.department) },
    { key: 'shift', header: 'Смена', render: (r) => shiftLabel(r.shift) },
    { key: 'hired', header: 'Принят', render: (r) => orTBC(r.hired_on) },
    { key: 'contract', header: 'Договор до', render: (r) => orTBC(r.contract_until) },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<Employee>
        kicker={t('group.admin')}
        title={t('mod.hr')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/hr/${r.id}`}
        exportKey="hr"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по ФИО и должности"
        onCreate={mayManage ? () => router.push('/hr/new') : undefined}
        createLabel={t('create')}
        isLoading={employees.isLoading}
        error={employees.isError ? { message: 'Не удалось загрузить список сотрудников' } : null}
        emptyTitle="Сотрудников нет"
        emptyBody="Добавьте первого сотрудника, чтобы вести кадровый учёт."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

/** The shift labels the client uses. `null` is not "no shift" — many staff are
 *  not on a rota at all — so it renders as «уточняется» like any other unknown. */
function shiftLabel(shift: string | null | undefined): string {
  switch (shift) {
    case 'day':
      return 'Дневная';
    case 'night':
      return 'Ночная';
    case 'rotating':
      return 'Сменная';
    default:
      return orTBC(null);
  }
}

/** Contracts ending within 30 days — the same horizon the alerts service uses,
 *  so the KPI and the notification bell cannot disagree. */
function expiringSoon(rows: Employee[]): number {
  const horizon = new Date();
  horizon.setDate(horizon.getDate() + 30);
  return rows.filter(
    (r) => r.contract_until && new Date(r.contract_until) <= horizon && r.status.key === 'active',
  ).length;
}
