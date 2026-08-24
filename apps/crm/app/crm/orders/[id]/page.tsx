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
import { useConfirmSalesOrder, useSalesOrder } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { SalesOrder, SalesOrderLine } from '@samari/types';

/**
 * Заказ клиента — sales order detail.
 *
 * Confirmation is the gate: Go checks that every line's batch is `released` and
 * reserves the stock in the same transaction. `useConfirmSalesOrder` had been
 * defined and wired to that endpoint since T25 and called by nothing, so the
 * check existed and no human could ever trigger it.
 */
export default function SalesOrderDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'crm', 'manage');
  const order = useSalesOrder(params.id);
  const detail = order.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel="Заказы клиентов"
        moduleHref="/crm/orders"
        isLoading={order.isLoading}
        error={
          order.isError
            ? { status: order.error instanceof ApiError ? order.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel="Заказы клиентов"
            moduleHref="/crm/orders"
            recordLabel={detail.so_no}
            title={detail.customer_name}
            identifier={detail.so_no}
            status={<StatusTag status={detail.status} />}
            actions={<ConfirmAction order={detail} mayManage={mayManage} />}
            groups={groupsFor(detail)}
            related={<Lines lines={detail.lines} />}
            activity={<ActivityPanel resource="crm" resourceId={detail.id} />}
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

function groupsFor(so: SalesOrder): FieldGroup[] {
  return [
    {
      title: 'Заказ клиента',
      fields: [
        { label: 'Номер', value: so.so_no },
        {
          label: 'Клиент',
          value: (
            <Link href={`/crm/${so.customer_id}`} className="hover:underline">
              {so.customer_name}
            </Link>
          ),
        },
        { label: 'Дата заказа', value: orTBC(so.ordered_on) },
        { label: 'Сумма', value: <span className="tabular-nums">{so.total} с.</span> },
      ],
    },
  ];
}

function ConfirmAction({ order, mayManage }: { order: SalesOrder; mayManage: boolean }) {
  const confirm = useConfirmSalesOrder(order.id);
  const [error, setError] = useState<string | null>(null);

  if (!mayManage || order.status.key !== 'draft') return null;

  return (
    <div className="flex flex-col items-end gap-1">
      <button
        type="button"
        className="btn btn-primary"
        disabled={confirm.isPending}
        data-testid="confirm-order"
        onClick={async () => {
          setError(null);
          try {
            await confirm.mutateAsync(undefined);
          } catch (e) {
            // Usually "batch not released" — the message names the actual line.
            setError(e instanceof Error ? e.message : 'Не удалось подтвердить заказ');
          }
        }}
      >
        Подтвердить
      </button>
      {error && (
        <span className="text-[12px]" role="alert" data-testid="confirm-error">
          {error}
        </span>
      )}
    </div>
  );
}

function Lines({ lines }: { lines: SalesOrderLine[] }) {
  const columns: RelatedColumn<SalesOrderLine>[] = [
    { key: 'sku', header: 'Артикул', render: (r) => r.sku },
    { key: 'item', header: 'Товар', render: (r) => r.item_name },
    {
      key: 'batch',
      header: 'Партия',
      render: (r) =>
        r.batch_id ? (
          <Link href={`/quality/${r.batch_id}`} className="hover:underline">
            {r.batch_no}
          </Link>
        ) : (
          orTBC(r.batch_no)
        ),
    },
    { key: 'qty', header: 'Количество', numeric: true, render: (r) => r.qty },
    { key: 'price', header: 'Цена', numeric: true, render: (r) => `${r.unit_price} с.` },
    { key: 'total', header: 'Сумма', numeric: true, render: (r) => `${r.line_total} с.` },
  ];

  return (
    <RelatedTable<SalesOrderLine>
      title="Позиции заказа"
      columns={columns}
      rows={lines}
      rowKey={(r) => r.id}
      emptyLabel="Позиций нет."
    />
  );
}
