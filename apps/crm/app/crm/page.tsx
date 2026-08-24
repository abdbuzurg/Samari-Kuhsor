'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useCRMKPIs, useCustomers } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { CustomerRow } from '@samari/types';

/**
 * CRM и продажи — the customer register (docs/05-MODULES.md:179).
 *
 * Rebuilt in R13. What shipped before was a sales-order table wearing this
 * module's name: none of the six specified columns matched, none of the four
 * specified KPIs existed, and the pipeline was absent entirely because nothing
 * had ever written a deal.
 *
 * Specified columns: Клиент · Тип · Регион · Статус · Сумма · Менеджер.
 * The pipeline board lives at /crm/pipeline; sales orders at /crm/orders.
 */

const TYPES: Record<string, string> = {
  distributor: 'Дистрибьютор',
  wholesale: 'Опт',
  retail: 'Розница',
};

export default function CustomersPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'crm', 'manage');

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const customers = useCustomers({ q: search, page });
  const kpiData = useCRMKPIs();
  const rows = customers.data?.data ?? [];
  const meta = customers.data?.meta;

  const k = kpiData.data;
  const kpis: KPI[] = [
    { label: 'Новые лиды', value: k ? String(k.new_leads) : '—' },
    { label: 'Открытые сделки', value: k ? String(k.open_deals) : '—' },
    {
      // Null, not 0: a pipeline with nothing decided has no conversion rate, and
      // 0% reads as total failure rather than "nothing has closed yet".
      label: 'Конверсия',
      value: k?.conversion == null ? 'уточняется' : `${k.conversion}%`,
    },
    { label: 'Просроченные задачи', value: k ? String(k.overdue_tasks) : '—' },
  ];

  const columns: Column<CustomerRow>[] = [
    { key: 'name', header: 'Клиент', render: (r) => r.name },
    {
      key: 'type',
      header: 'Тип',
      render: (r) => (r.customer_type ? (TYPES[r.customer_type] ?? r.customer_type) : 'уточняется'),
    },
    { key: 'region', header: 'Регион', render: (r) => orTBC(r.region) },
    {
      key: 'status',
      header: 'Статус',
      render: (r) => (r.lead_status ? <StatusTag status={r.lead_status} /> : <span className="muted">—</span>),
    },
    {
      key: 'amount',
      header: 'Сумма',
      numeric: true,
      render: (r) =>
        Number(r.open_amount) > 0 ? (
          <span className="tabular-nums">{r.open_amount} с.</span>
        ) : (
          <span className="muted">—</span>
        ),
    },
    { key: 'deals', header: 'Сделок', numeric: true, render: (r) => r.open_deals },
  ];

  return (
    <AppShell>
      <div className="flex gap-2 mb-4">
        <Link href="/crm/pipeline" className="btn btn-secondary">
          Воронка сделок
        </Link>
        <Link href="/crm/orders" className="btn btn-secondary">
          Заказы клиентов
        </Link>
        <Link href="/crm/tasks" className="btn btn-secondary">
          Задачи
        </Link>
      </div>

      <ListView<CustomerRow>
        kicker={t('group.sales')}
        title={t('mod.crm')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/crm/${r.id}`}
        exportKey="customers"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по названию, региону и контакту"
        onCreate={mayManage ? () => router.push('/crm/new') : undefined}
        createLabel={t('create')}
        isLoading={customers.isLoading}
        error={customers.isError ? { message: 'Не удалось загрузить клиентов' } : null}
        emptyTitle="Клиентов нет"
        emptyBody="Клиент появляется здесь после конвертации обращения с сайта или создаётся вручную."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
