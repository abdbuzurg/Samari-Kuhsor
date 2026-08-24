'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import { WorkflowActions } from '@/components/WorkflowActions';
import { useBatchDetail, useIssueBatchQR, useRecordQualityTest } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { BatchStatusEvent, QualityTest, StockBalanceRow } from '@samari/types';

/**
 * Качество и безопасность — batch detail (docs/05-MODULES.md:149).
 *
 * "batch header · all tests with results and inspectors · status history with
 * who decided · where the stock is now".
 *
 * This screen is ToR §8 acceptance condition 5 — "Quality staff can quarantine
 * and release finished goods" — and until R03 it did not exist in any form. The
 * endpoint, the 32-case transition matrix and the RBAC behind it were all built
 * and tested months ago; there was simply no client code, not even a hook.
 *
 * Nothing here re-implements the matrix. `allowed_transitions` is computed by
 * the server from the same rules it enforces, so the buttons cannot drift from
 * what is actually permitted.
 */

const STATUS_LABELS: Record<string, string> = {
  quarantine: 'На карантин',
  released: 'Выпустить',
  rejected: 'Забраковать',
  in_production: 'В производство',
};

/** Mirrors the CHECK constraint (docs/05-MODULES.md:144). */
const TEST_TYPES: Array<{ value: string; label: string }> = [
  { value: 'ph', label: 'pH' },
  { value: 'microbiology', label: 'Микробиология' },
  { value: 'brix', label: 'Брикс' },
  { value: 'viscosity', label: 'Вязкость' },
  { value: 'metal_detection', label: 'Металлодетекция' },
  { value: 'organoleptic', label: 'Органолептика' },
];

export default function BatchDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();

  const mayManage = can(session.data?.permissions, 'quality', 'manage');
  const batch = useBatchDetail(params.id);

  const detail = batch.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.quality')}
        moduleHref="/quality"
        isLoading={batch.isLoading}
        error={
          batch.isError
            ? { status: batch.error instanceof ApiError ? batch.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.quality')}
            moduleHref="/quality"
            recordLabel={detail.batch.batch_no}
            title={detail.item_name}
            identifier={`${detail.batch.batch_no} · ${detail.sku}`}
            status={<StatusTag status={detail.batch.status} />}
            actions={
              <>
                <Link
                  href={`/print/batch/${detail.batch.id}`}
                  className="btn btn-secondary"
                  target="_blank"
                >
                  Паспорт качества
                </Link>
                <WorkflowActions
                endpoint={`/api/batches/${detail.batch.id}/transition`}
                invalidate={['batch-detail', 'quality/batches', 'stock']}
                allowed={detail.allowed_transitions}
                labels={STATUS_LABELS}
                disabled={!mayManage}
                // Only a recall carries a mandatory reason (quality.go:78). The
                // server refuses without one regardless; collecting it here just
                // means the refusal never has to be shown.
                  reasonFor={(to) => detail.batch.status.key === 'released' && to === 'rejected'}
                />
              </>
            }
            groups={groupsFor(detail)}
            related={
              <>
                <TestRecorder
                  batchId={detail.batch.id}
                  tests={detail.tests}
                  mayManage={mayManage}
                />
                <QRBand
                  batchId={detail.batch.id}
                  payload={detail.batch.qr_payload}
                  issuedAt={detail.batch.qr_issued_at}
                  mayManage={mayManage}
                />
                <StatusHistory history={detail.history} />
                <StockPositions stock={detail.stock} />
              </>
            }
            activity={<ActivityPanel resource="quality" resourceId={detail.batch.id} />}
            footer={{
              createdAt: formatDateTime(detail.batch.created_at),
              // A batch carries no updated_at: its mutable history is the status
              // event chain, which is rendered in full above.
              updatedAt: formatDateTime(detail.batch.created_at),
              version: detail.batch.version,
            }}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(detail: NonNullable<ReturnType<typeof useBatchDetail>['data']>): FieldGroup[] {
  const failed = detail.tests.filter((x) => x.result.level === 'danger').length;
  return [
    {
      title: 'Партия',
      fields: [
        { label: 'Номер партии', value: detail.batch.batch_no },
        { label: 'Артикул', value: detail.sku },
        { label: 'Товар', value: detail.item_name },
        { label: 'Произведена', value: orTBC(detail.batch.produced_on) },
        { label: 'Годен до', value: orTBC(detail.batch.expires_on) },
        {
          label: 'Проверок',
          value:
            detail.tests.length === 0 ? (
              'уточняется'
            ) : failed > 0 ? (
              <span>
                {detail.tests.length} · <strong>не пройдено: {failed}</strong>
              </span>
            ) : (
              String(detail.tests.length)
            ),
        },
      ],
    },
  ];
}

/**
 * Tests, plus the form that adds one.
 *
 * The form lives beside the table rather than behind a modal because recording a
 * result is the most frequent thing anyone does on this screen, and a lab
 * technician entering six readings should not open six dialogs.
 */
function TestRecorder({
  batchId,
  tests,
  mayManage,
}: {
  batchId: string;
  tests: QualityTest[];
  mayManage: boolean;
}) {
  const record = useRecordQualityTest(batchId);
  const [open, setOpen] = useState(false);
  const [testType, setTestType] = useState(TEST_TYPES[0].value);
  const [resultValue, setResultValue] = useState('');
  const [passed, setPassed] = useState(true);
  const [notes, setNotes] = useState('');
  const [error, setError] = useState<string | null>(null);

  const columns: RelatedColumn<QualityTest>[] = [
    {
      key: 'type',
      header: 'Проверка',
      render: (r) => TEST_TYPES.find((x) => x.value === r.test_type)?.label ?? r.test_type,
    },
    { key: 'result', header: 'Результат', render: (r) => <StatusTag status={r.result} /> },
    { key: 'value', header: 'Значение', render: (r) => orTBC(r.result_value) },
    { key: 'inspector', header: 'Кто проверял', render: (r) => orTBC(r.inspector) },
    {
      key: 'when',
      header: 'Когда',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.tested_at)}</span>,
    },
  ];

  async function submit() {
    setError(null);
    try {
      await record.mutateAsync({
        test_type: testType,
        result_value: resultValue.trim() || undefined,
        passed,
        notes: notes.trim() || undefined,
      });
      setOpen(false);
      setResultValue('');
      setNotes('');
      setPassed(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось записать проверку');
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<QualityTest>
        title="Лабораторные проверки"
        columns={columns}
        rows={tests}
        rowKey={(r) => r.id}
        emptyLabel="Проверок ещё не было."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-test-form"
            >
              {open ? 'Отмена' : 'Записать проверку'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="test-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Тип проверки
              <select
                className="input"
                value={testType}
                onChange={(e) => setTestType(e.target.value)}
                aria-label="Тип проверки"
              >
                {TEST_TYPES.map((x) => (
                  <option key={x.value} value={x.value}>
                    {x.label}
                  </option>
                ))}
              </select>
            </label>

            <label className="flex flex-col gap-1 text-[12px] muted">
              Значение
              <input
                className="input"
                value={resultValue}
                onChange={(e) => setResultValue(e.target.value)}
                aria-label="Значение"
              />
            </label>
          </div>

          <label className="flex items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              checked={passed}
              onChange={(e) => setPassed(e.target.checked)}
              aria-label="Проверка пройдена"
            />
            Проверка пройдена
          </label>

          <label className="flex flex-col gap-1 text-[12px] muted">
            Примечание
            <textarea
              className="input"
              rows={2}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              aria-label="Примечание"
            />
          </label>

          {error && (
            <p className="text-[12px]" role="alert" data-testid="test-error">
              {error}
            </p>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={record.isPending}
              onClick={() => void submit()}
              data-testid="save-test"
            >
              Сохранить
            </button>
          </div>
        </section>
      )}
    </div>
  );
}

/**
 * Who decided what, and why.
 *
 * `batch_status_events` has no version and no deleted_at — it is immutable
 * evidence, which is exactly what makes the traceability claim defensible. There
 * is no edit control here because there is no query behind one.
 */
function StatusHistory({ history }: { history: BatchStatusEvent[] }) {
  const columns: RelatedColumn<BatchStatusEvent>[] = [
    {
      key: 'move',
      header: 'Переход',
      render: (r) => (
        <span className="flex items-center gap-1.5 flex-wrap">
          {r.from_status ? <StatusTag status={r.from_status} /> : <span className="muted">—</span>}
          <span className="muted">→</span>
          <StatusTag status={r.to_status} />
        </span>
      ),
    },
    { key: 'who', header: 'Кто решил', render: (r) => orTBC(r.decider_name) },
    { key: 'reason', header: 'Причина', render: (r) => orTBC(r.reason) },
    {
      key: 'when',
      header: 'Когда',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.occurred_at)}</span>,
    },
  ];

  return (
    <RelatedTable<BatchStatusEvent>
      title="История решений"
      columns={columns}
      rows={history}
      rowKey={(r) => r.id}
      emptyLabel="Решений по партии ещё не принимали."
    />
  );
}

/** Where the stock actually is — the last question a recall has to answer. */
function StockPositions({ stock }: { stock: StockBalanceRow[] }) {
  const columns: RelatedColumn<StockBalanceRow>[] = [
    { key: 'location', header: 'Локация', render: (r) => `${r.location_code} · ${r.location_zone}` },
    {
      key: 'qty',
      header: 'Остаток',
      numeric: true,
      render: (r) => (
        <span className="tabular-nums">
          {r.on_hand} {r.base_uom}
        </span>
      ),
    },
  ];

  return (
    <RelatedTable<StockBalanceRow>
      title="Где находится"
      columns={columns}
      rows={stock}
      rowKey={(r) => `${r.location_id}`}
      emptyLabel="Остатков по этой партии нет."
    />
  );
}

/**
 * The QR payload printed onto the wrapper.
 *
 * D11: wrappers are ordered months in advance, so this is issued long before the
 * batch is produced — and once issued it never changes, because the printed
 * codes would stop resolving. The endpoint has existed since T15 and had no BFF
 * route until R18 tried to drive it.
 */
function QRBand({
  batchId,
  payload,
  issuedAt,
  mayManage,
}: {
  batchId: string;
  payload?: string | null;
  issuedAt?: string | null;
  mayManage: boolean;
}) {
  const issue = useIssueBatchQR(batchId);
  const [error, setError] = useState<string | null>(null);

  return (
    <section className="card p-4 flex flex-col gap-2" data-testid="qr-band">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
          QR-код партии
        </h2>
        {mayManage && !payload && (
          <button
            type="button"
            className="btn btn-secondary"
            disabled={issue.isPending}
            data-testid="issue-qr"
            onClick={async () => {
              setError(null);
              try {
                await issue.mutateAsync();
              } catch (e) {
                setError(e instanceof Error ? e.message : 'Не удалось выпустить QR-код');
              }
            }}
          >
            Выпустить QR-код
          </button>
        )}
      </div>

      {payload ? (
        <div className="text-[12.5px]" data-testid="qr-payload">
          <div className="break-all">{payload}</div>
          <div className="muted mt-0.5">Выпущен: {formatDateTime(issuedAt)}</div>
          <div className="muted mt-0.5">
            Код выпускается один раз и не меняется — обёртки печатаются заранее.
          </div>
        </div>
      ) : (
        <p className="muted text-[12.5px]" data-testid="qr-absent">
          QR-код ещё не выпущен.
        </p>
      )}

      {error && (
        <p className="text-[12px]" role="alert" data-testid="qr-error">
          {error}
        </p>
      )}
    </section>
  );
}
