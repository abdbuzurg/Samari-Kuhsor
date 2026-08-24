'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column } from '@/components/ListView';
import { actionLabel, resourceLabel, shortId } from '@/lib/labels';
import { useAuditLog } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import type { AuditEntry } from '@samari/types';

/**
 * Журнал действий — the audit viewer (docs/04-RBAC.md §6).
 *
 * Read-only, and not by convention: `audit_log` has no UPDATE and no DELETE
 * query anywhere in the backend and no `deleted_at` column to tombstone with.
 * There is no route that could edit an entry because there is no query that
 * could. That is what makes it evidence rather than a log.
 */
export default function AuditPage() {
  const t = useTranslations();
  const [resource, setResource] = useState('');
  const [page, setPage] = useState(1);

  const audit = useAuditLog({ resource: resource || undefined, page });
  const rows = audit.data?.data ?? [];
  const meta = audit.data?.meta;

  const columns: Column<AuditEntry>[] = [
    {
      key: 'when',
      header: 'Когда',
      render: (r) => (
        <span className="tabular-nums">{new Date(r.occurred_at).toLocaleString('ru-RU')}</span>
      ),
    },
    {
      key: 'actor',
      header: 'Кто',
      // A public enquiry has no actor. «Система» is truthful; a blank cell reads
      // as missing data, and inventing a user would put a name against an action
      // nobody in the company took.
      render: (r) => r.actor_name ?? 'Система',
    },
    { key: 'action', header: 'Действие', render: (r) => actionLabel(r.action) },
    { key: 'resource', header: 'Модуль', render: (r) => resourceLabel(r.resource) },
    {
      key: 'target',
      header: 'Объект',
      render: (r) => (
        <span className="tabular-nums text-[12px]">{shortId(r.resource_id)}</span>
      ),
    },
    { key: 'ip', header: 'IP', render: (r) => orTBC(r.ip) },
  ];

  return (
    <AppShell>
      <ListView<AuditEntry>
        kicker={t('group.admin')}
        title="Журнал действий"
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        exportKey="audit"
        search={resource}
        onSearchChange={(v) => {
          setResource(v);
          setPage(1);
        }}
        searchPlaceholder="Фильтр по модулю: items, inventory, quality…"
        createLabel={t('create')}
        isLoading={audit.isLoading}
        error={audit.isError ? { message: 'Не удалось загрузить журнал действий' } : null}
        emptyTitle="Записей нет"
        emptyBody="Журнал заполняется автоматически при любом изменении данных."
        noMatchLabel="Записей не найдено для модуля"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

