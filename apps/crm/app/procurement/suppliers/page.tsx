'use client';

import Link from 'next/link';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { ListView, type Column, type KPI } from '@/components/ListView';
import { useCreateSupplier, useSuppliers } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can } from '@/lib/session';
import type { Supplier } from '@samari/types';

/**
 * Поставщики.
 *
 * A register inside Закупки rather than a module of its own: the nav is exactly
 * thirteen modules (docs/05-MODULES.md §1) and suppliers are not one of them.
 */
export default function SuppliersPage() {
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'procurement', 'manage');
  const create = useCreateSupplier();

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [region, setRegion] = useState('');
  const [contact, setContact] = useState('');
  const [error, setError] = useState<string | null>(null);

  const suppliers = useSuppliers({ q: search, page });
  const rows = suppliers.data?.data ?? [];
  const meta = suppliers.data?.meta;

  const kpis: KPI[] = [{ label: 'Поставщиков', value: meta ? String(meta.total) : '—' }];

  const columns: Column<Supplier>[] = [
    { key: 'name', header: 'Поставщик', render: (r) => r.name },
    { key: 'region', header: 'Регион', render: (r) => orTBC(r.region) },
    { key: 'contact', header: 'Контакт', render: (r) => orTBC(r.contact) },
    { key: 'tax', header: 'ИНН', render: (r) => orTBC(r.tax_id) },
  ];

  return (
    <AppShell>
      <div className="flex gap-2 mb-4">
        <Link href="/procurement" className="btn btn-secondary">
          Заказы поставщикам
        </Link>
      </div>

      {open && (
        <section className="card p-4 flex flex-col gap-3 mb-4 max-w-2xl" data-testid="supplier-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Название
              <input
                className="input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                aria-label="Название поставщика"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Регион
              <input
                className="input"
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                aria-label="Регион поставщика"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Контакт
              <input
                className="input"
                value={contact}
                onChange={(e) => setContact(e.target.value)}
                aria-label="Контакт поставщика"
              />
            </label>
          </div>
          {error && (
            <p className="text-[12px]" role="alert" data-testid="supplier-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={create.isPending || !name.trim()}
              data-testid="save-supplier"
              onClick={async () => {
                setError(null);
                try {
                  await create.mutateAsync({
                    name: name.trim(),
                    region: region.trim() || undefined,
                    contact: contact.trim() || undefined,
                  });
                  setOpen(false);
                  setName('');
                  setRegion('');
                  setContact('');
                } catch (e) {
                  setError(e instanceof Error ? e.message : 'Не удалось сохранить поставщика');
                }
              }}
            >
              Сохранить
            </button>
          </div>
        </section>
      )}

      <ListView<Supplier>
        kicker="Операции"
        title="Поставщики"
        kpis={kpis}
        columns={columns}
        rows={rows}
        rowKey={(r) => r.id}
        search={search}
        onSearchChange={(v) => {
          setSearch(v);
          setPage(1);
        }}
        searchPlaceholder="Поиск по названию и региону"
        onCreate={mayManage ? () => setOpen((v) => !v) : undefined}
        createLabel="Добавить поставщика"
        isLoading={suppliers.isLoading}
        error={suppliers.isError ? { message: 'Не удалось загрузить поставщиков' } : null}
        emptyTitle="Поставщиков нет"
        emptyBody="Добавьте поставщика, чтобы оформить заказ."
        noMatchLabel="Ничего не найдено по запросу"
        meta={meta}
        onPageChange={setPage}
      />
    </AppShell>
  );
}
