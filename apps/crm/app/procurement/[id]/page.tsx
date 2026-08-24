'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import { WorkflowActions } from '@/components/WorkflowActions';
import { useLocations, usePurchaseOrder, useReceivePurchaseOrder } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { PurchaseOrder, PurchaseOrderLine } from '@samari/types';

/**
 * Закупки — purchase order detail (docs/05-MODULES.md:199).
 *
 * "PO header · supplier · line items · receipt history · linked stock movements".
 *
 * Receiving posts `goods_receipt` movements in the same transaction, so the
 * warehouse balance moves the moment the lorry is unloaded rather than when
 * somebody remembers to key it in.
 */

const PO_LABELS: Record<string, string> = {
  draft: 'В черновик',
  approval: 'На согласование',
  confirmed: 'Подтвердить',
  in_transit: 'В пути',
  receiving: 'К приёмке',
  closed: 'Закрыть',
  cancelled: 'Отменить',
};

export default function PurchaseOrderDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'procurement', 'manage');
  const order = usePurchaseOrder(params.id);
  const detail = order.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.procurement')}
        moduleHref="/procurement"
        isLoading={order.isLoading}
        error={
          order.isError
            ? { status: order.error instanceof ApiError ? order.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.procurement')}
            moduleHref="/procurement"
            recordLabel={detail.po_no}
            title={detail.supplier_name}
            identifier={detail.po_no}
            status={<StatusTag status={detail.status} />}
            actions={
              <>
                <Link
                  href={`/print/purchase-order/${detail.id}`}
                  className="btn btn-secondary"
                  target="_blank"
                >
                  Печать
                </Link>
                <WorkflowActions
                endpoint={`/api/purchase-orders/${detail.id}/transition`}
                invalidate={['purchase-orders']}
                allowed={detail.allowed_transitions}
                labels={PO_LABELS}
                  disabled={!mayManage}
                />
              </>
            }
            groups={groupsFor(detail)}
            related={
              <>
                <Lines lines={detail.lines} />
                <Receiver order={detail} mayManage={mayManage} />
              </>
            }
            activity={<ActivityPanel resource="procurement" resourceId={detail.id} />}
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

function groupsFor(po: PurchaseOrder): FieldGroup[] {
  return [
    {
      title: 'Заказ поставщику',
      fields: [
        { label: 'Номер', value: po.po_no },
        { label: 'Поставщик', value: po.supplier_name },
        { label: 'Ожидается', value: orTBC(po.expected_at) },
        { label: 'Сумма', value: <span className="tabular-nums">{po.total} с.</span> },
      ],
    },
  ];
}

function Lines({ lines }: { lines: PurchaseOrderLine[] }) {
  const columns: RelatedColumn<PurchaseOrderLine>[] = [
    { key: 'sku', header: 'Артикул', render: (r) => r.sku },
    { key: 'item', header: 'Позиция', render: (r) => r.item_name },
    { key: 'qty', header: 'Заказано', numeric: true, render: (r) => r.qty },
    {
      key: 'received',
      header: 'Принято',
      numeric: true,
      // Outstanding quantity is the question this table exists to answer, so a
      // partially received line is called out rather than left to arithmetic.
      render: (r) =>
        Number(r.received_qty) < Number(r.qty) ? (
          <strong className="tabular-nums">{r.received_qty}</strong>
        ) : (
          <span className="tabular-nums">{r.received_qty}</span>
        ),
    },
    { key: 'price', header: 'Цена', numeric: true, render: (r) => `${r.unit_price} с.` },
    { key: 'total', header: 'Сумма', numeric: true, render: (r) => `${r.line_total} с.` },
  ];

  return (
    <RelatedTable<PurchaseOrderLine>
      title="Позиции заказа"
      columns={columns}
      rows={lines}
      rowKey={(r) => r.id}
      emptyLabel="Позиций нет."
    />
  );
}

/**
 * Goods receipt.
 *
 * Received quantity is checked against what was ordered, and receiving against a
 * closed order is refused — both in Go. The form collects a quantity per line and
 * the destination location, because stock has to land somewhere specific.
 */
function Receiver({ order, mayManage }: { order: PurchaseOrder; mayManage: boolean }) {
  const receive = useReceivePurchaseOrder(order.id);
  const locations = useLocations();
  const [open, setOpen] = useState(false);
  const [locationId, setLocationId] = useState('');
  const [qtys, setQtys] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  const canReceive = mayManage && ['confirmed', 'in_transit', 'receiving'].includes(order.status.key);

  if (!canReceive) return null;

  async function submit() {
    setError(null);
    const lines = order.lines
      .filter((l) => (qtys[l.id] ?? '').trim())
      .map((l) => ({ po_line_id: l.id, qty: qtys[l.id].trim() }));
    if (lines.length === 0) {
      setError('Укажите принятое количество хотя бы по одной позиции');
      return;
    }
    try {
      await receive.mutateAsync({ location_id: locationId, lines });
      setOpen(false);
      setQtys({});
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось оформить приёмку');
    }
  }

  return (
    <section className="card p-4 flex flex-col gap-3" data-testid="receipt-section">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
          Приёмка
        </h2>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={() => setOpen((v) => !v)}
          data-testid="toggle-receipt"
        >
          {open ? 'Отмена' : 'Оформить приёмку'}
        </button>
      </div>

      {open && (
        <div className="flex flex-col gap-3" data-testid="receipt-form">
          <label className="flex flex-col gap-1 text-[12px] muted">
            Локация
            <select
              className="input"
              value={locationId}
              onChange={(e) => setLocationId(e.target.value)}
              aria-label="Локация"
            >
              <option value="">— выберите —</option>
              {(locations.data ?? []).map((l) => (
                <option key={l.id} value={l.id}>
                  {l.code} · {l.name}
                </option>
              ))}
            </select>
          </label>

          {order.lines.map((line) => (
            <label key={line.id} className="flex flex-col gap-1 text-[12px] muted">
              {`${line.sku} · ${line.item_name} — заказано ${line.qty}, принято ${line.received_qty}`}
              <input
                className="input"
                value={qtys[line.id] ?? ''}
                onChange={(e) => setQtys((q) => ({ ...q, [line.id]: e.target.value }))}
                aria-label={`Принято по ${line.sku}`}
                inputMode="decimal"
              />
            </label>
          ))}

          {error && (
            <p className="text-[12px]" role="alert" data-testid="receipt-error">
              {error}
            </p>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={receive.isPending || !locationId}
              onClick={() => void submit()}
              data-testid="save-receipt"
            >
              Принять
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
