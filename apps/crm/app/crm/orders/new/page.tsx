'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { useCreateSalesOrder, useCustomers, useQualityBatches } from '@/lib/operations';

/**
 * Новый заказ клиента.
 *
 * Lines name a BATCH, not just a product: confirming the order checks every
 * line's batch is `released` and reserves the stock. Offering only released
 * batches here means the check almost never has to fire — but it still fires,
 * in Go, because hiding is not enforcement.
 */
export default function NewSalesOrderPage() {
  const router = useRouter();
  const create = useCreateSalesOrder();
  const customers = useCustomers({ page: 1 });
  const released = useQualityBatches({ status: 'released' });

  const [soNo, setSoNo] = useState('');
  const [customerId, setCustomerId] = useState('');
  const [orderedOn, setOrderedOn] = useState('');
  const [lines, setLines] = useState([{ batch_id: '', qty: '', unit_price: '' }]);
  const [error, setError] = useState<string | null>(null);

  const batches = released.data?.data ?? [];
  const complete = lines.filter((l) => l.batch_id && l.qty.trim() && l.unit_price.trim());
  const canSave = soNo.trim() && customerId && complete.length > 0;

  return (
    <AppShell>
      <h1 className="text-[27px] leading-[1.05] mb-4" style={{ fontFamily: 'var(--font-heading)' }}>
        Новый заказ клиента
      </h1>

      <section className="card p-5 flex flex-col gap-4 max-w-3xl" data-testid="so-form">
        <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
          <label className="flex flex-col gap-1 text-[12px] muted">
            Номер заказа
            <input
              className="input"
              value={soNo}
              onChange={(e) => setSoNo(e.target.value)}
              aria-label="Номер заказа"
              placeholder="SO-0101"
            />
          </label>
          <label className="flex flex-col gap-1 text-[12px] muted">
            Клиент
            <select
              className="input"
              value={customerId}
              onChange={(e) => setCustomerId(e.target.value)}
              aria-label="Клиент"
            >
              <option value="">— выберите —</option>
              {(customers.data?.data ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[12px] muted">
            Дата заказа
            <input
              className="input"
              type="date"
              value={orderedOn}
              onChange={(e) => setOrderedOn(e.target.value)}
              aria-label="Дата заказа"
            />
          </label>
        </div>

        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
              Позиции
            </h2>
            <button
              type="button"
              className="btn btn-secondary"
              data-testid="add-so-line"
              onClick={() => setLines((l) => [...l, { batch_id: '', qty: '', unit_price: '' }])}
            >
              Добавить позицию
            </button>
          </div>

          {batches.length === 0 && !released.isLoading && (
            <p className="muted text-[12px]" data-testid="no-released-batches">
              Выпущенных партий нет. Партию сначала выпускает Качество.
            </p>
          )}

          {lines.map((line, i) => (
            <div key={i} className="grid gap-2 grid-cols-1 sm:grid-cols-3" data-testid="so-line">
              <select
                className="input"
                value={line.batch_id}
                aria-label={`Партия ${i + 1}`}
                onChange={(e) =>
                  setLines((l) => l.map((x, j) => (j === i ? { ...x, batch_id: e.target.value } : x)))
                }
              >
                <option value="">— партия —</option>
                {batches.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.batch_no} · {b.item_name}
                  </option>
                ))}
              </select>
              <input
                className="input"
                value={line.qty}
                inputMode="decimal"
                placeholder="Количество"
                aria-label={`Количество ${i + 1}`}
                onChange={(e) =>
                  setLines((l) => l.map((x, j) => (j === i ? { ...x, qty: e.target.value } : x)))
                }
              />
              <input
                className="input"
                value={line.unit_price}
                inputMode="decimal"
                placeholder="Цена"
                aria-label={`Цена ${i + 1}`}
                onChange={(e) =>
                  setLines((l) =>
                    l.map((x, j) => (j === i ? { ...x, unit_price: e.target.value } : x)),
                  )
                }
              />
            </div>
          ))}
        </div>

        {error && (
          <p className="text-[12px]" role="alert" data-testid="so-error">
            {error}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => router.push('/crm/orders')}
          >
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!canSave || create.isPending}
            data-testid="so-save"
            onClick={async () => {
              setError(null);
              try {
                const created = (await create.mutateAsync({
                  so_no: soNo.trim(),
                  customer_id: customerId,
                  ordered_on: orderedOn || undefined,
                  lines: complete.map((l) => {
                    const batch = batches.find((b) => b.id === l.batch_id);
                    return {
                      item_id: batch?.item_id ?? '',
                      batch_id: l.batch_id,
                      qty: l.qty.trim(),
                      unit_price: l.unit_price.trim(),
                    };
                  }),
                })) as { id?: string };
                router.push(created?.id ? `/crm/orders/${created.id}` : '/crm/orders');
              } catch (e) {
                setError(e instanceof Error ? e.message : 'Не удалось создать заказ');
              }
            }}
          >
            Создать
          </button>
        </div>
      </section>
    </AppShell>
  );
}
