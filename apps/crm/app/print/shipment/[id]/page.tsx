'use client';

import { useParams } from 'next/navigation';

import { PrintDocument, PrintFields, PrintSignatures } from '@/components/PrintDocument';
import { useShipment } from '@/lib/operations';
import { orTBC } from '@/lib/resource';

/**
 * Товарно-транспортная накладная — the delivery note that travels with the lorry.
 *
 * Every line names its batch. That is what makes the paper in the driver's cab
 * match the traceability chain in the system, and it is the document a customer
 * quotes back when they complain.
 */
export default function ShipmentPrintPage() {
  const params = useParams<{ id: string }>();
  const trip = useShipment(params.id);
  const shipment = trip.data;

  if (trip.isLoading) return <p className="p-6 muted text-[13px]">Загрузка…</p>;
  if (trip.isError || !shipment) return <p className="p-6 text-[13px]">Рейс не найден.</p>;

  return (
    <PrintDocument title="Товарно-транспортная накладная" subtitle={`Рейс ${shipment.trip_no}`}>
      <PrintFields
        fields={[
          { label: 'Номер рейса', value: shipment.trip_no },
          { label: 'Маршрут', value: `${orTBC(shipment.route_from)} → ${orTBC(shipment.route_to)}` },
          { label: 'Водитель', value: orTBC(shipment.driver_name) },
          { label: 'Транспорт', value: orTBC(shipment.vehicle_plate) },
          { label: 'Статус', value: shipment.status.label },
        ]}
      />

      <table className="mb-4" data-testid="shipment-lines">
        <thead>
          <tr>
            <th>Артикул</th>
            <th>Наименование</th>
            <th>Партия</th>
            <th className="num">Количество</th>
          </tr>
        </thead>
        <tbody>
          {shipment.lines.map((line) => (
            <tr key={line.id}>
              <td>{line.sku}</td>
              <td>{line.item_name}</td>
              <td>{line.batch_no}</td>
              <td className="num">{line.qty}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <PrintSignatures roles={['Отпустил', 'Водитель', 'Принял']} />
    </PrintDocument>
  );
}
