'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { useCreatePurchaseOrder, useItemsForPicker, useSuppliers } from '@/lib/operations';

/**
 * Новый заказ поставщику.
 *
 * Not the shared CreateForm: an order is a header plus a variable number of
 * lines, and a flat field grid cannot express that. The order is created as a
 * draft — `approval` and everything past it are reached through the ladder on
 * the order's own screen, where `procurement:approve` is enforced.
 */
export default function NewPurchaseOrderPage() {
  const router = useRouter();
  const create = useCreatePurchaseOrder();
  const suppliers = useSuppliers({ page: 1 });
  const items = useItemsForPicker();

  const [poNo, setPoNo] = useState('');
  const [supplierId, setSupplierId] = useState('');
  const [expectedAt, setExpectedAt] = useState('');
  const [lines, setLines] = useState([{ item_id: '', qty: '', unit_price: '' }]);
  const [error, setError] = useState<string | null>(null);

  const complete = lines.filter((l) => l.item_id && l.qty.trim() && l.unit_price.trim());
  const canSave = poNo.trim() && supplierId && complete.length > 0;

  return (
    <AppShell>
      <h1 className="text-[27px] leading-[1.05] mb-4" style={{ fontFamily: 'var(--font-heading)' }}>
        Новый заказ поставщику
      </h1>

      <section className="card p-5 flex flex-col gap-4 max-w-3xl" data-testid="po-form">
        <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
          <label className="flex flex-col gap-1 text-[12px] muted">
            Номер заказа
            <input
              className="input"
              value={poNo}
              onChange={(e) => setPoNo(e.target.value)}
              aria-label="Номер заказа"
              placeholder="PO-0031"
            />
          </label>
          <label className="flex flex-col gap-1 text-[12px] muted">
            Поставщик
            <select
              className="input"
              value={supplierId}
              onChange={(e) => setSupplierId(e.target.value)}
              aria-label="Поставщик"
            >
              <option value="">— выберите —</option>
              {(suppliers.data?.data ?? []).map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[12px] muted">
            Ожидается
            <input
              className="input"
              type="date"
              value={expectedAt}
              onChange={(e) => setExpectedAt(e.target.value)}
              aria-label="Ожидается"
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
              data-testid="add-line"
              onClick={() => setLines((l) => [...l, { item_id: '', qty: '', unit_price: '' }])}
            >
              Добавить позицию
            </button>
          </div>

          {lines.map((line, i) => (
            <div key={i} className="grid gap-2 grid-cols-1 sm:grid-cols-3" data-testid="po-line">
              <select
                className="input"
                value={line.item_id}
                aria-label={`Позиция ${i + 1}`}
                onChange={(e) =>
                  setLines((l) => l.map((x, j) => (j === i ? { ...x, item_id: e.target.value } : x)))
                }
              >
                <option value="">— товар —</option>
                {(items.data ?? []).map((it) => (
                  <option key={it.id} value={it.id}>
                    {it.sku} · {it.name}
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
          <p className="text-[12px]" role="alert" data-testid="po-error">
            {error}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-secondary" onClick={() => router.push('/procurement')}>
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!canSave || create.isPending}
            data-testid="po-save"
            onClick={async () => {
              setError(null);
              try {
                const created = (await create.mutateAsync({
                  po_no: poNo.trim(),
                  supplier_id: supplierId,
                  expected_at: expectedAt || undefined,
                  lines: complete.map((l) => ({
                    item_id: l.item_id,
                    qty: l.qty.trim(),
                    unit_price: l.unit_price.trim(),
                  })),
                })) as { id?: string };
                router.push(created?.id ? `/procurement/${created.id}` : '/procurement');
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
