'use client';

import { useRouter } from 'next/navigation';
import { useLocale, useTranslations } from 'next-intl';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { StatusTag } from '@/components/StatusTag';
import { useItems, orTBC } from '@/lib/items';
import { useSession, can } from '@/lib/session';
import type { ItemListRow } from '@samari/types';

/**
 * Товары и цены — list view.
 *
 * Columns and KPIs come from docs/05-MODULES.md §4, which takes them from the
 * approved prototype: SKU · Наименование · Категория · Упаковка · Цена ·
 * Срок годн. · Статус.
 */
export default function ItemsPage() {
  const t = useTranslations();
  const locale = useLocale();
  const router = useRouter();

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);

  const session = useSession();
  const mayManage = can(session.data?.permissions, 'items', 'manage');

  const items = useItems({ q: search, page, locale });
  const rows = items.data?.data ?? [];
  const meta = items.data?.meta;

  // KPIs from docs/05-MODULES.md:84. "Языки каталога" is 3 by definition — the
  // platform ships ru/tg/en (D10) — not a count of what happens to be filled in.
  const kpis: KPI[] = [
    { label: 'Всего SKU', value: meta ? String(meta.total) : '—' },
    {
      label: 'Активных',
      value: String(rows.filter((r) => r.status.key === 'active').length),
    },
    { label: 'Языки каталога', value: '3' },
    { label: 'Средняя цена', value: averagePrice(rows) },
  ];

  const columns: Column<ItemListRow>[] = [
    { key: 'sku', header: 'SKU', render: (r) => <span className="tabular-nums">{r.sku}</span> },
    { key: 'name', header: 'Наименование', render: (r) => r.name },
    { key: 'category', header: 'Категория', render: (r) => orTBC(r.category) },
    {
      key: 'packaging',
      header: 'Упаковка',
      render: (r) => (r.packaging_codes.length ? r.packaging_codes.join(' · ') : orTBC(null)),
    },
    {
      key: 'price',
      header: 'Цена',
      numeric: true,
      // Money arrives as a string and stays one. Formatting it through a Number
      // would reintroduce exactly the float the contract forbids.
      render: (r) => (r.current_price ? `${r.current_price.amount} c.` : orTBC(null)),
    },
    {
      key: 'shelf_life',
      header: 'Срок годн.',
      numeric: true,
      // Null until lab-verified: «уточняется», never an empty cell that reads as
      // "no shelf life" (docs/02-SCHEMA.md:176).
      render: (r) => (r.shelf_life_days ? `${r.shelf_life_days} дн` : orTBC(null)),
    },
    {
      key: 'status',
      header: 'Статус',
      render: (r) => <StatusTag status={r.status} />,
    },
  ];

  return (
    <AppShell>
      <ListView<ItemListRow>
        kicker={t('group.sales')}
        title={t('mod.items')}
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        rowHref={(r) => `/items/${r.id}`}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1); // a new search starts at page 1, or results look empty
        }}
        searchPlaceholder="Поиск по артикулу и наименованию"
        onCreate={mayManage ? () => router.push('/items/new') : undefined}
        createLabel={t('create')}
        isLoading={items.isLoading}
        error={items.isError ? { message: 'Не удалось загрузить список товаров' } : null}
        emptyTitle="Товаров пока нет"
        emptyBody="Добавьте первую позицию каталога, чтобы начать работу."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}

/** Mean of the prices actually present, in somoni. Items without a price are
 *  excluded rather than counted as zero, which would drag the average down and
 *  misreport the catalogue. */
function averagePrice(rows: ItemListRow[]): string {
  const priced = rows.filter((r) => r.current_price);
  if (priced.length === 0) return '—';
  const total = priced.reduce((sum, r) => sum + Number(r.current_price!.amount), 0);
  return `${(total / priced.length).toFixed(2)} c.`;
}
