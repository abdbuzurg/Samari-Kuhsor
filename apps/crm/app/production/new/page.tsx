'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateManufacturingOrder, useItemsForPicker } from '@/lib/operations';

/**
 * Новый заказ на производство.
 *
 * MO ↔ batch is 1:1, so the batch number is part of creating the order rather
 * than something attached later. Go creates the batch in the same transaction.
 */
export default function NewManufacturingOrderPage() {
  const create = useCreateManufacturingOrder();
  const items = useItemsForPicker();

  return (
    <CreateForm
      moduleLabel="Производство"
      moduleHref="/production"
      title="Новый заказ на производство"
      fields={[
        { name: 'mo_no', label: 'Номер заказа', required: true, placeholder: 'MO-1041' },
        {
          name: 'item_id',
          label: 'Товар',
          type: 'select',
          required: true,
          options: (items.data ?? []).map((i) => ({ value: i.id, label: `${i.sku} · ${i.name}` })),
        },
        { name: 'batch_no', label: 'Номер партии', required: true, placeholder: 'B-2617' },
        { name: 'planned_qty', label: 'Плановое количество', type: 'number', required: true },
        { name: 'scheduled_for', label: 'Запланировано на', type: 'date' },
        { name: 'line', label: 'Линия' },
      ]}
      columns={3}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          mo_no: v.mo_no.trim(),
          item_id: v.item_id,
          batch_no: v.batch_no.trim(),
          planned_qty: v.planned_qty.trim(),
          scheduled_for: v.scheduled_for || undefined,
          line: v.line.trim() || undefined,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
