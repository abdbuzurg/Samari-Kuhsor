'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateShipment } from '@/lib/operations';

/** Новый рейс. Loading happens on the trip's own screen, once it exists. */
export default function NewShipmentPage() {
  const create = useCreateShipment();

  return (
    <CreateForm
      moduleLabel="Логистика"
      moduleHref="/logistics"
      title="Новый рейс"
      fields={[
        { name: 'trip_no', label: 'Номер рейса', required: true, placeholder: 'TR-0077' },
        { name: 'route_from', label: 'Откуда', placeholder: 'Хорог' },
        { name: 'route_to', label: 'Куда', placeholder: 'Душанбе' },
        // An operational cost, not a finance record (02-SCHEMA.md:334).
        { name: 'transport_cost', label: 'Стоимость перевозки, с.', type: 'number' },
      ]}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          trip_no: v.trip_no.trim(),
          route_from: v.route_from.trim() || undefined,
          route_to: v.route_to.trim() || undefined,
          transport_cost: v.transport_cost.trim() || undefined,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
