'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useInquiries } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { Inquiry } from '@samari/types';

/**
 * Интеграция с сайтом — list view. Columns from docs/05-MODULES.md §11.
 *
 * These rows are the only records in the system created by someone with no
 * account. The reference number is what the visitor holds, so it leads.
 */
export default function InquiriesPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const inquiries = useInquiries({ q: search, page });
  const rows = inquiries.data?.data ?? [];
  const meta = inquiries.data?.meta;

  const complaints = rows.filter((r) => r.type.key === 'complaint').length;
  const kpis: KPI[] = [
    { label: 'Обращений', value: meta ? String(meta.total) : '—' },
    { label: 'Новых', value: String(rows.filter((r) => r.status.key === 'new').length) },
    {
      label: 'Жалоб',
      value: String(complaints),
      // A complaint may mean product in the field is wrong; it is the entry point
      // to the traceability workflow, so it is called out rather than counted
      // silently alongside sales enquiries.
      delta: complaints > 0 ? { text: 'требует расследования', direction: 'danger' } : undefined,
    },
    { label: 'Создано лидов', value: String(rows.filter((r) => r.status.key === 'lead_created').length) },
  ];

  const columns: Column<Inquiry>[] = [
    {
      key: 'reference',
      header: 'Номер',
      render: (r) => <span className="tabular-nums">{r.reference_no}</span>,
    },
    { key: 'type', header: 'Тип', render: (r) => <StatusTag status={r.type} /> },
    { key: 'name', header: 'Отправитель', render: (r) => r.name },
    { key: 'company', header: 'Компания', render: (r) => orTBC(r.company) },
    { key: 'contact', header: 'Контакт', render: (r) => r.contact },
    { key: 'batch', header: 'Партия', render: (r) => orTBC(r.batch_no) },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<Inquiry>
        kicker={t('group.sales')}
        title={t('mod.inquiries')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/inquiries/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру, имени и компании"
        createLabel={t('create')}
        isLoading={inquiries.isLoading}
        error={inquiries.isError ? { message: 'Не удалось загрузить обращения' } : null}
        emptyTitle="Обращений с сайта нет"
        emptyBody="Заявки с публичного сайта появятся здесь автоматически."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
