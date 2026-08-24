'use client';

import { useParams } from 'next/navigation';

import { PrintDocument, PrintFields, PrintSignatures } from '@/components/PrintDocument';
import { usePurchaseOrder } from '@/lib/operations';
import { orTBC } from '@/lib/resource';

/** Заказ поставщику — the purchase order as sent to a supplier. */
export default function PurchaseOrderPrintPage() {
  const params = useParams<{ id: string }>();
  const order = usePurchaseOrder(params.id);
  const po = order.data;

  if (order.isLoading) return <p className="p-6 muted text-[13px]">Загрузка…</p>;
  if (order.isError || !po) return <p className="p-6 text-[13px]">Заказ не найден.</p>;

  return (
    <PrintDocument title="Заказ поставщику" subtitle={`${po.po_no} · ${po.supplier_name}`}>
      <PrintFields
        fields={[
          { label: 'Номер заказа', value: po.po_no },
          { label: 'Поставщик', value: po.supplier_name },
          { label: 'Ожидаемая поставка', value: orTBC(po.expected_at) },
          { label: 'Статус', value: po.status.label },
        ]}
      />

      <table className="mb-4" data-testid="po-lines">
        <thead>
          <tr>
            <th>Артикул</th>
            <th>Наименование</th>
            <th className="num">Количество</th>
            <th className="num">Цена</th>
            <th className="num">Сумма</th>
          </tr>
        </thead>
        <tbody>
          {po.lines.map((line) => (
            <tr key={line.id}>
              <td>{line.sku}</td>
              <td>{line.item_name}</td>
              <td className="num">{line.qty}</td>
              <td className="num">{line.unit_price}</td>
              <td className="num">{line.line_total}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr>
            <th colSpan={4}>Итого</th>
            <th className="num" data-testid="po-total">
              {po.total} с.
            </th>
          </tr>
        </tfoot>
      </table>

      <PrintSignatures roles={['Закупки', 'Утвердил']} />
    </PrintDocument>
  );
}
