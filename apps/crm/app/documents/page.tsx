'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useDocuments } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { Document } from '@samari/types';

/**
 * Документы — list view. Columns from docs/05-MODULES.md §14.
 *
 * Ordered by expiry, because an expired certificate is a regulatory exposure
 * rather than a filing inconvenience.
 */
export default function DocumentsPage() {
  const t = useTranslations();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const documents = useDocuments({ q: search, page });
  const rows = documents.data?.data ?? [];
  const meta = documents.data?.meta;

  const expired = rows.filter((r) => r.status.key === 'expired').length;
  const kpis: KPI[] = [
    { label: 'Документов', value: meta ? String(meta.total) : '—' },
    { label: 'Действуют', value: String(rows.filter((r) => r.status.key === 'active').length) },
    {
      label: 'На согласовании',
      value: String(rows.filter((r) => r.status.key === 'approval').length),
    },
    {
      label: 'Истекли',
      value: String(expired),
      delta: expired > 0 ? { text: 'требуют продления', direction: 'danger' } : undefined,
    },
  ];

  const columns: Column<Document>[] = [
    { key: 'doc_no', header: 'Номер', render: (r) => <span className="tabular-nums">{r.doc_no}</span> },
    { key: 'title', header: 'Название', render: (r) => r.title },
    { key: 'type', header: 'Тип', render: (r) => orTBC(r.doc_type) },
    { key: 'owner', header: 'Ответственный', render: (r) => orTBC(r.owner_name) },
    { key: 'valid_until', header: 'Действует до', render: (r) => orTBC(r.valid_until) },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      <ListView<Document>
        kicker={t('group.admin')}
        title={t('mod.documents')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/documents/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру и названию"
        createLabel={t('create')}
        isLoading={documents.isLoading}
        error={documents.isError ? { message: 'Не удалось загрузить документы' } : null}
        emptyTitle="Документов нет"
        emptyBody="Добавьте сертификаты, паспорта качества и разрешительные документы."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
