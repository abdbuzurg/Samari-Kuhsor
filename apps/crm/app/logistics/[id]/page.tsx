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
import { useLoadShipment, useQualityBatches, useShipment } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { Shipment, ShipmentLine } from '@samari/types';

/**
 * Логистика — trip detail.
 *
 * The loading list is the point of this screen. Go refuses a line whose batch is
 * not `released`, because a lorry leaving Хорог with quarantined product is the
 * failure the entire quality chain exists to prevent. This page does not
 * re-implement that check — it offers released batches, and shows the server's
 * refusal verbatim if anything slips past.
 */
export default function ShipmentDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'logistics', 'manage');
  const trip = useShipment(params.id);
  const detail = trip.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.logistics')}
        moduleHref="/logistics"
        isLoading={trip.isLoading}
        error={
          trip.isError
            ? { status: trip.error instanceof ApiError ? trip.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.logistics')}
            moduleHref="/logistics"
            recordLabel={detail.trip_no}
            title={`Рейс ${detail.trip_no}`}
            identifier={routeLabel(detail)}
            status={<StatusTag status={detail.status} />}
            actions={
              <Link
                href={`/print/shipment/${detail.id}`}
                className="btn btn-secondary"
                target="_blank"
              >
                Накладная
              </Link>
            }
            groups={groupsFor(detail)}
            related={<Loader id={detail.id} lines={detail.lines} mayManage={mayManage} />}
            activity={<ActivityPanel resource="logistics" resourceId={detail.id} />}
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

/** «Хорог → Душанбе», or a single «уточняется» when neither end is set. */
function routeLabel(trip: Shipment): string {
  if (!trip.route_from && !trip.route_to) return 'уточняется';
  return `${trip.route_from ?? '—'} → ${trip.route_to ?? '—'}`;
}

function groupsFor(trip: Shipment): FieldGroup[] {
  return [
    {
      title: 'Рейс',
      fields: [
        { label: 'Номер рейса', value: trip.trip_no },
        { label: 'Откуда', value: orTBC(trip.route_from) },
        { label: 'Куда', value: orTBC(trip.route_to) },
        { label: 'Водитель', value: orTBC(trip.driver_name) },
        { label: 'Транспорт', value: orTBC(trip.vehicle_plate) },
        {
          // An operational cost, NOT a finance record (02-SCHEMA.md:334).
          // Финансы и бюджет is out of scope and this must not become the thin
          // end of it.
          label: 'Стоимость перевозки',
          value: trip.transport_cost ? `${trip.transport_cost} с.` : 'уточняется',
        },
      ],
    },
  ];
}

function Loader({
  id,
  lines,
  mayManage,
}: {
  id: string;
  lines: ShipmentLine[];
  mayManage: boolean;
}) {
  const load = useLoadShipment(id);
  // Only released batches can legally be loaded, so only released batches are
  // offered. The server still refuses anything else.
  const released = useQualityBatches({ status: 'released' });
  const [open, setOpen] = useState(false);
  const [batchId, setBatchId] = useState('');
  const [qty, setQty] = useState('');
  const [error, setError] = useState<string | null>(null);

  const options = released.data?.data ?? [];

  const columns: RelatedColumn<ShipmentLine>[] = [
    { key: 'sku', header: 'Артикул', render: (r) => r.sku },
    { key: 'item', header: 'Товар', render: (r) => r.item_name },
    {
      key: 'batch',
      header: 'Партия',
      render: (r) => (
        <Link href={`/quality/${r.batch_id}`} className="hover:underline">
          {r.batch_no}
        </Link>
      ),
    },
    { key: 'qty', header: 'Количество', numeric: true, render: (r) => r.qty },
  ];

  async function submit() {
    setError(null);
    try {
      // item_id travels with the batch: a line names both, and deriving it
      // from the chosen batch is what stops the two disagreeing.
      const chosen = options.find((b) => b.id === batchId);
      await load.mutateAsync({
        item_id: chosen?.item_id ?? '',
        batch_id: batchId,
        qty: qty.trim(),
      });
      setOpen(false);
      setBatchId('');
      setQty('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось загрузить партию');
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<ShipmentLine>
        title="Погрузочный лист"
        columns={columns}
        rows={lines}
        rowKey={(r) => r.id}
        emptyLabel="Рейс пока пустой."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-load-form"
            >
              {open ? 'Отмена' : 'Загрузить партию'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="load-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Партия (выпущенные)
              <select
                className="input"
                value={batchId}
                onChange={(e) => setBatchId(e.target.value)}
                aria-label="Партия"
              >
                <option value="">— выберите —</option>
                {options.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.batch_no} · {b.item_name}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Количество
              <input
                className="input"
                value={qty}
                onChange={(e) => setQty(e.target.value)}
                aria-label="Количество"
                inputMode="decimal"
              />
            </label>
          </div>

          {options.length === 0 && !released.isLoading && (
            <p className="muted text-[12px]" data-testid="no-released">
              Выпущенных партий нет. Партию сначала выпускает Качество.
            </p>
          )}

          {error && (
            <p className="text-[12px]" role="alert" data-testid="load-error">
              {error}
            </p>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={load.isPending || !batchId || !qty.trim()}
              onClick={() => void submit()}
              data-testid="save-load"
            >
              Загрузить
            </button>
          </div>
        </section>
      )}
    </div>
  );
}
