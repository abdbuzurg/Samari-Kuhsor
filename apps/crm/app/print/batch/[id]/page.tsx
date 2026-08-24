'use client';

import { useParams } from 'next/navigation';

import { PrintDocument, PrintFields, PrintSignatures } from '@/components/PrintDocument';
import { useBatchDetail } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { formatDateTime } from '@/lib/format';

/**
 * Паспорт качества партии — the batch certificate.
 *
 * The document with actual regulatory weight: it names the batch, every test
 * performed against it, and the user who released it. That last fact is the
 * whole point — a certificate that does not name a decision-maker is not
 * evidence of a decision.
 *
 * Renders only for a RELEASED batch. Printing a certificate for something still
 * in quarantine would produce a document asserting something untrue, and the
 * refusal is deliberately explicit rather than a blank page.
 */
export default function BatchCertificatePage() {
  const params = useParams<{ id: string }>();
  const batch = useBatchDetail(params.id);
  const detail = batch.data;

  if (batch.isLoading) {
    return <p className="p-6 muted text-[13px]">Загрузка…</p>;
  }
  if (batch.isError || !detail) {
    return <p className="p-6 text-[13px]">Партия не найдена.</p>;
  }

  const released = detail.batch.status.key === 'released';
  const release = detail.history.find((e) => e.to_status.key === 'released');

  return (
    <PrintDocument
      title="Паспорт качества"
      subtitle={`Партия ${detail.batch.batch_no} · ${detail.item_name}`}
    >
      {!released && (
        <p
          className="text-[13px] mb-5 p-3"
          style={{ border: '1px solid #c0341c', color: '#c0341c' }}
          data-testid="not-released"
        >
          Партия не выпущена. Паспорт качества оформляется только после решения о
          выпуске.
        </p>
      )}

      <PrintFields
        fields={[
          { label: 'Номер партии', value: detail.batch.batch_no },
          { label: 'Артикул', value: detail.sku },
          { label: 'Наименование', value: detail.item_name },
          { label: 'Дата производства', value: orTBC(detail.batch.produced_on) },
          { label: 'Годен до', value: orTBC(detail.batch.expires_on) },
          { label: 'Статус', value: detail.batch.status.label },
        ]}
      />

      <h2 className="text-[15px] mb-2" style={{ fontFamily: 'var(--font-heading)' }}>
        Результаты испытаний
      </h2>
      {detail.tests.length === 0 ? (
        <p className="text-[13px] mb-6" style={{ color: '#5a6152' }}>
          Испытания не проводились.
        </p>
      ) : (
        <table className="mb-6" data-testid="certificate-tests">
          <thead>
            <tr>
              <th>Показатель</th>
              <th>Значение</th>
              <th>Результат</th>
              <th>Кто проверял</th>
              <th>Дата</th>
            </tr>
          </thead>
          <tbody>
            {detail.tests.map((test) => (
              <tr key={test.id}>
                <td>{test.test_type}</td>
                <td>{orTBC(test.result_value)}</td>
                <td>{test.result.label}</td>
                <td>{orTBC(test.inspector)}</td>
                <td>{formatDateTime(test.tested_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {release && (
        <PrintFields
          fields={[
            { label: 'Решение о выпуске принял', value: orTBC(release.decider_name) },
            { label: 'Дата решения', value: formatDateTime(release.occurred_at) },
          ]}
        />
      )}

      <PrintSignatures roles={['Начальник лаборатории', 'Ответственный за выпуск']} />
    </PrintDocument>
  );
}
