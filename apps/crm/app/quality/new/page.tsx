'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateBatch, useItemsForPicker } from '@/lib/operations';

/**
 * Новая партия.
 *
 * Batches are created BEFORE the plant produces anything: the QR payload is
 * printed onto wrappers ordered months in advance (D11). A batch number that
 * only appears once product exists is a batch number that arrives too late.
 */
export default function NewBatchPage() {
  const create = useCreateBatch();
  const items = useItemsForPicker();

  return (
    <CreateForm
      moduleLabel="Качество и безопасность"
      moduleHref="/quality"
      title="Новая партия"
      fields={[
        { name: 'batch_no', label: 'Номер партии', required: true, placeholder: 'B-2617' },
        {
          name: 'item_id',
          label: 'Товар',
          type: 'select',
          required: true,
          options: (items.data ?? []).map((i) => ({ value: i.id, label: `${i.sku} · ${i.name}` })),
        },
        { name: 'produced_on', label: 'Дата производства', type: 'date' },
        { name: 'expires_on', label: 'Годен до', type: 'date' },
      ]}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          batch_no: v.batch_no.trim(),
          item_id: v.item_id,
          produced_on: v.produced_on || undefined,
          expires_on: v.expires_on || undefined,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
