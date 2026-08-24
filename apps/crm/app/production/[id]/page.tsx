'use client';

import { useParams } from 'next/navigation';
import Link from 'next/link';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import { useCompleteOrder, useManufacturingOrder, useRecordEntry } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { ManufacturingOrder, ProductionEntry } from '@samari/types';

/**
 * Производство — manufacturing order detail (docs/05-MODULES.md:131).
 *
 * "order header · batch · shift entries (append-only log of good/scrap/downtime)".
 *
 * Completing an order posts its output into a QUARANTINE location and moves the
 * batch to `quarantine`. It does not make the batch sellable — only Качество can,
 * which is why the completion button here says «Завершить», never «Выпустить».
 */
export default function ManufacturingOrderPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'production', 'manage');
  const order = useManufacturingOrder(params.id);
  const detail = order.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.production')}
        moduleHref="/production"
        isLoading={order.isLoading}
        error={
          order.isError
            ? { status: order.error instanceof ApiError ? order.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.production')}
            moduleHref="/production"
            recordLabel={detail.mo_no}
            title={detail.item_name}
            identifier={`${detail.mo_no} · ${detail.sku}`}
            status={<StatusTag status={detail.status} />}
            actions={<CompleteAction order={detail} mayManage={mayManage} />}
            groups={groupsFor(detail)}
            related={
              <EntryRecorder id={detail.id} entries={detail.entries} mayManage={mayManage} />
            }
            activity={<ActivityPanel resource="production" resourceId={detail.id} />}
            footer={{
              createdAt: formatDateTime(detail.created_at),
              updatedAt: formatDateTime(detail.created_at),
              version: detail.version,
            }}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(mo: ManufacturingOrder): FieldGroup[] {
  return [
    {
      title: 'Заказ на производство',
      fields: [
        { label: 'Номер', value: mo.mo_no },
        { label: 'Линия', value: orTBC(mo.line) },
        { label: 'Запланировано на', value: orTBC(mo.scheduled_for) },
        {
          label: 'Партия',
          value: mo.batch_id ? (
            // The batch is where this order's output ends up, and the only place
            // it can be released. Linking is the whole traceability chain.
            <Link href={`/quality/${mo.batch_id}`} className="hover:underline">
              {mo.batch_no}
            </Link>
          ) : (
            orTBC(mo.batch_no)
          ),
        },
      ],
    },
    {
      title: 'Выпуск',
      fields: [
        { label: 'План', value: `${mo.planned_qty}` },
        { label: 'Годных', value: `${mo.good_qty}` },
        { label: 'Брак', value: `${mo.scrap_qty}` },
        { label: 'Прогресс', value: `${mo.progress}%` },
        { label: 'Простой, мин', value: String(mo.downtime_min) },
        // Yield is null, not 0, before anything has run — a line that has made
        // nothing has no yield, and 0% reads as a catastrophe.
        { label: 'Выход', value: mo.yield_percent == null ? 'уточняется' : `${mo.yield_percent}%` },
      ],
    },
  ];
}

/**
 * Completion.
 *
 * Not a WorkflowActions ladder: an MO has exactly one forward move and the
 * server exposes it as a dedicated sub-resource rather than a transition matrix.
 */
function CompleteAction({ order, mayManage }: { order: ManufacturingOrder; mayManage: boolean }) {
  const complete = useCompleteOrder(order.id);
  const [error, setError] = useState<string | null>(null);

  if (!mayManage || order.status.key === 'done' || order.status.key === 'cancelled') {
    return null;
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <button
        type="button"
        className="btn btn-primary"
        disabled={complete.isPending}
        data-testid="complete-order"
        onClick={async () => {
          setError(null);
          try {
            await complete.mutateAsync(undefined);
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось завершить заказ');
          }
        }}
      >
        Завершить
      </button>
      {error && (
        <span className="text-[12px]" role="alert" data-testid="complete-error">
          {error}
        </span>
      )}
    </div>
  );
}

/**
 * Shift entries.
 *
 * Append-only. Output, yield and downtime are SUMS over these rows and never
 * columns, so a wrong entry is corrected by adding another, not by editing one.
 */
function EntryRecorder({
  id,
  entries,
  mayManage,
}: {
  id: string;
  entries: ProductionEntry[];
  mayManage: boolean;
}) {
  const record = useRecordEntry(id);
  const [open, setOpen] = useState(false);
  const [good, setGood] = useState('');
  const [scrap, setScrap] = useState('');
  const [downtime, setDowntime] = useState('');
  const [note, setNote] = useState('');
  const [error, setError] = useState<string | null>(null);

  const columns: RelatedColumn<ProductionEntry>[] = [
    {
      key: 'when',
      header: 'Смена',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.recorded_at)}</span>,
    },
    { key: 'good', header: 'Годных', numeric: true, render: (r) => r.good_qty },
    { key: 'scrap', header: 'Брак', numeric: true, render: (r) => r.scrap_qty },
    { key: 'downtime', header: 'Простой, мин', numeric: true, render: (r) => r.downtime_min },
    { key: 'who', header: 'Кто записал', render: (r) => orTBC(r.recorded_by) },
    { key: 'note', header: 'Примечание', render: (r) => orTBC(r.note) },
  ];

  async function submit() {
    setError(null);
    try {
      await record.mutateAsync({
        good_qty: good.trim() || '0',
        scrap_qty: scrap.trim() || '0',
        downtime_min: downtime.trim() ? Number(downtime) : 0,
        note: note.trim() || undefined,
      });
      setOpen(false);
      setGood('');
      setScrap('');
      setDowntime('');
      setNote('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось записать смену');
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<ProductionEntry>
        title="Записи по сменам"
        columns={columns}
        rows={entries}
        rowKey={(r) => r.id}
        emptyLabel="Записей по сменам ещё нет."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-entry-form"
            >
              {open ? 'Отмена' : 'Записать смену'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="entry-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Годных
              <input
                className="input"
                value={good}
                onChange={(e) => setGood(e.target.value)}
                aria-label="Годных"
                inputMode="decimal"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Брак
              <input
                className="input"
                value={scrap}
                onChange={(e) => setScrap(e.target.value)}
                aria-label="Брак"
                inputMode="decimal"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Простой, мин
              <input
                className="input"
                value={downtime}
                onChange={(e) => setDowntime(e.target.value)}
                aria-label="Простой, мин"
                inputMode="numeric"
              />
            </label>
          </div>

          <label className="flex flex-col gap-1 text-[12px] muted">
            Примечание
            <textarea
              className="input"
              rows={2}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              aria-label="Примечание"
            />
          </label>

          {error && (
            <p className="text-[12px]" role="alert" data-testid="entry-error">
              {error}
            </p>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={record.isPending}
              onClick={() => void submit()}
              data-testid="save-entry"
            >
              Сохранить
            </button>
          </div>
        </section>
      )}
    </div>
  );
}
