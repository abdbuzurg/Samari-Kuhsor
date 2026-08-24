'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useQualityBatches } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { BatchListRow } from '@samari/types';

/**
 * Качество и безопасность — list view. Columns from docs/05-MODULES.md §7.
 *
 * The server orders quarantined batches first, because those are the only ones
 * waiting on a human decision. This page does not re-sort them.
 */
export default function QualityPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'quality', 'manage');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const batches = useQualityBatches({ q: search, page });
  const rows = batches.data?.data ?? [];
  const meta = batches.data?.meta;

  const quarantined = rows.filter((r) => r.status.key === 'quarantine').length;
  const kpis: KPI[] = [
    { label: 'Партий', value: meta ? String(meta.total) : '—' },
    {
      label: 'На карантине',
      value: String(quarantined),
      delta: quarantined > 0 ? { text: 'ожидают решения', direction: 'warn' } : undefined,
    },
    { label: 'Выпущено', value: String(rows.filter((r) => r.status.key === 'released').length) },
    {
      label: 'Отклонено',
      value: String(rows.filter((r) => r.status.key === 'rejected').length),
    },
  ];

  const columns: Column<BatchListRow>[] = [
    {
      key: 'batch_no',
      header: 'Партия',
      render: (r) => <span className="tabular-nums">{r.batch_no}</span>,
    },
    { key: 'name', header: 'Продукция', render: (r) => r.item_name },
    { key: 'produced', header: 'Произведена', render: (r) => orTBC(r.produced_on) },
    { key: 'expires', header: 'Годен до', render: (r) => orTBC(r.expires_on) },
    {
      key: 'tests',
      header: 'Испытания',
      numeric: true,
      // A failed test is shown next to the count rather than only in the detail
      // view: the whole point of this list is spotting the batch that needs
      // attention without opening twenty pages.
      render: (r) => (
        <span className="tabular-nums">
          {r.test_count}
          {r.failed_count > 0 ? ` (${r.failed_count} — не соотв.)` : ''}
        </span>
      ),
    },
    { key: 'status', header: 'Статус', render: (r) => <StatusTag status={r.status} /> },
  ];

  return (
    <AppShell>
      {/* The printer handoff (D11). Wrappers are ordered months in advance, so
          this is used long before the plant produces anything. */}
      <div className="flex gap-2 mb-4">
        <a className="btn btn-secondary" href="/api/batches/qr-export" download>
          Экспорт QR-кодов для типографии
        </a>
      </div>

      <ListView<BatchListRow>
        kicker={t('group.ops')}
        title={t('mod.quality')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/quality/${r.id}`}
        exportKey="quality"
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по номеру партии и продукции"
        onCreate={mayManage ? () => router.push('/quality/new') : undefined}
        createLabel={t('create')}
        isLoading={batches.isLoading}
        error={batches.isError ? { message: 'Не удалось загрузить партии' } : null}
        emptyTitle="Партий нет"
        emptyBody="Партии появятся после первого выпуска продукции."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
