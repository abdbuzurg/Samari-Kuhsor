'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateAsset } from '@/lib/operations';

/** Новое оборудование. */
export default function NewAssetPage() {
  const create = useCreateAsset();

  return (
    <CreateForm
      moduleLabel="Оборудование и ТО"
      moduleHref="/equipment"
      title="Новое оборудование"
      fields={[
        { name: 'asset_no', label: 'Инв. номер', required: true, placeholder: 'EQ-047' },
        { name: 'name', label: 'Наименование', required: true },
        { name: 'asset_type', label: 'Тип' },
        { name: 'line', label: 'Линия' },
        { name: 'commissioned_on', label: 'Введено в эксплуатацию', type: 'date' },
        { name: 'warranty_until', label: 'Гарантия до', type: 'date' },
      ]}
      columns={3}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          asset_no: v.asset_no.trim(),
          name: v.name.trim(),
          asset_type: v.asset_type.trim() || undefined,
          line: v.line.trim() || undefined,
          commissioned_on: v.commissioned_on || undefined,
          warranty_until: v.warranty_until || undefined,
          status: 'running',
          version: 0,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
