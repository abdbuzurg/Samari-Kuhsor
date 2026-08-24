'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateEmployee } from '@/lib/operations';

/** Новый сотрудник. Contract expiry is what the register is ordered by. */
export default function NewEmployeePage() {
  const create = useCreateEmployee();

  return (
    <CreateForm
      moduleLabel="Персонал"
      moduleHref="/hr"
      title="Новый сотрудник"
      fields={[
        { name: 'full_name', label: 'ФИО', required: true, wide: true },
        {
          name: 'shift',
          label: 'Смена',
          type: 'select',
          options: [
            { value: 'day', label: 'Дневная' },
            { value: 'night', label: 'Ночная' },
            { value: 'rotating', label: 'Сменный график' },
          ],
        },
        { name: 'hired_on', label: 'Принят', type: 'date' },
        { name: 'contract_until', label: 'Договор до', type: 'date' },
      ]}
      columns={3}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          full_name: v.full_name.trim(),
          shift: v.shift || undefined,
          hired_on: v.hired_on || undefined,
          contract_until: v.contract_until || undefined,
          status: 'active',
          version: 0,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
