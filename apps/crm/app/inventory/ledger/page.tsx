'use client';

import { useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import { useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import {
  useLocations,
  usePostMovement,
  useStock,
  useStockLedger,
  useTransferStock,
} from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { StockBalanceRow, StockMovementRow } from '@samari/types';

/**
 * Склад и запасы — one position's ledger (docs/05-MODULES.md:112).
 *
 * A position is (item, batch, location), which is why this route is addressed by
 * query parameters rather than a single id: there is no row with a primary key
 * behind it. `stock_balances` is a plain VIEW and every quantity on the register
 * is a SUM computed at read time (CLAUDE.md §4.2). This screen is what makes that
 * defensible — a figure of 480 can always be explained by the rows below it.
 *
 * **No form here offers an absolute quantity, and none ever may**
 * (05-MODULES.md:112). Every write is a DELTA. A correction is a compensating
 * entry; the row that was wrong is never edited and never tombstoned.
 */
export default function StockLedgerPage() {
  return (
    <Suspense fallback={<AppShell><p className="muted text-[13px]">Загрузка…</p></AppShell>}>
      <StockLedgerView />
    </Suspense>
  );
}

function StockLedgerView() {
  const t = useTranslations();
  const search = useSearchParams();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'inventory', 'manage');

  const itemId = search.get('item_id') ?? undefined;
  const batchId = search.get('batch_id');
  const locationId = search.get('location_id') ?? undefined;

  const ledger = useStockLedger({ itemId, batchId, locationId });
  // The position's own header row. Filtering the register by item is cheaper
  // than a dedicated endpoint and keeps one definition of "a position".
  const positions = useStock({ itemId });
  const position = (positions.data?.data ?? []).find(
    (p) => p.location_id === locationId && (p.batch_id ?? null) === (batchId ?? null),
  );

  const missingParams = !itemId || !locationId;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.inventory')}
        moduleHref="/inventory"
        isLoading={!missingParams && (ledger.isLoading || positions.isLoading)}
        error={
          missingParams
            ? { status: 404 }
            : ledger.isError
              ? { status: ledger.error instanceof ApiError ? ledger.error.status : undefined }
              : null
        }
      >
        <DetailView
          moduleLabel={t('mod.inventory')}
          moduleHref="/inventory"
          recordLabel={position ? position.sku : 'Позиция'}
          title={position?.item_name ?? 'Позиция склада'}
          identifier={
            position
              ? `${position.sku} · ${position.location_code}${position.batch_no ? ` · ${position.batch_no}` : ''}`
              : undefined
          }
          status={position ? <StatusTag status={position.status} /> : undefined}
          groups={groupsFor(position)}
          related={
            <div className="flex flex-col gap-3">
              <MovementForms
                itemId={itemId}
                batchId={batchId}
                locationId={locationId}
                mayManage={mayManage}
              />
              <LedgerTable rows={ledger.data?.data ?? []} />
            </div>
          }
        />
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(position: StockBalanceRow | undefined): FieldGroup[] {
  if (!position) {
    return [{ title: 'Позиция', fields: [{ label: 'Остаток', value: 'уточняется' }] }];
  }
  return [
    {
      title: 'Позиция',
      fields: [
        { label: 'Артикул', value: position.sku },
        { label: 'Товар', value: position.item_name },
        { label: 'Локация', value: `${position.location_code} · ${position.location_zone}` },
        { label: 'Партия', value: orTBC(position.batch_no) },
        { label: 'Годен до', value: orTBC(position.expires_on) },
        {
          label: 'Остаток',
          value: (
            <span className="tabular-nums">
              {position.on_hand} {position.base_uom}
            </span>
          ),
        },
        { label: 'Минимум', value: orTBC(position.min_qty) },
        { label: 'Последнее движение', value: formatDateTime(position.last_movement_at) },
      ],
    },
  ];
}

/** Reason codes, mirroring the CHECK constraint in migration 00002. */
const REASONS: Array<{ value: string; label: string }> = [
  { value: 'goods_receipt', label: 'Приёмка' },
  { value: 'production_output', label: 'Выпуск продукции' },
  { value: 'material_issue', label: 'Отпуск в производство' },
  { value: 'sale', label: 'Продажа' },
  { value: 'scrap', label: 'Списание' },
  { value: 'return', label: 'Возврат' },
  { value: 'adjustment', label: 'Корректировка' },
];

/** Reasons that take stock OUT. The sign is derived, never typed by the user. */
const OUTBOUND = new Set(['material_issue', 'sale', 'scrap']);

function MovementForms({
  itemId,
  batchId,
  locationId,
  mayManage,
}: {
  itemId?: string;
  batchId: string | null;
  locationId?: string;
  mayManage: boolean;
}) {
  const post = usePostMovement();
  const transfer = useTransferStock();
  const locations = useLocations();

  const [mode, setMode] = useState<'none' | 'movement' | 'transfer'>('none');
  const [reason, setReason] = useState(REASONS[0].value);
  const [qty, setQty] = useState('');
  const [toLocation, setToLocation] = useState('');
  const [note, setNote] = useState('');
  const [error, setError] = useState<string | null>(null);

  if (!mayManage || !itemId || !locationId) return null;

  async function submitMovement() {
    setError(null);
    const magnitude = qty.trim().replace(/^[+-]/, '');
    try {
      await post.mutateAsync({
        item_id: itemId!,
        batch_id: batchId ?? undefined,
        location_id: locationId!,
        // The user types a magnitude and picks a reason; the SIGN comes from the
        // reason. Letting them type "-80" would be a second way to express the
        // same thing and the first step towards "set stock to X".
        qty_delta: OUTBOUND.has(reason) ? `-${magnitude}` : magnitude,
        reason,
        note: note.trim() || undefined,
      });
      setMode('none');
      setQty('');
      setNote('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось провести движение');
    }
  }

  async function submitTransfer() {
    setError(null);
    try {
      await transfer.mutateAsync({
        item_id: itemId!,
        batch_id: batchId ?? undefined,
        from_location_id: locationId!,
        to_location_id: toLocation,
        qty: qty.trim(),
        note: note.trim() || undefined,
      });
      setMode('none');
      setQty('');
      setNote('');
      setToLocation('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось выполнить перемещение');
    }
  }

  return (
    <section className="card p-4 flex flex-col gap-3" data-testid="movement-forms">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-[15px]" style={{ fontFamily: 'var(--font-heading)' }}>
          Провести движение
        </h2>
        <div className="flex gap-1.5">
          <button
            type="button"
            className="btn btn-secondary"
            data-testid="toggle-movement"
            onClick={() => setMode((m) => (m === 'movement' ? 'none' : 'movement'))}
          >
            Движение
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            data-testid="toggle-transfer"
            onClick={() => setMode((m) => (m === 'transfer' ? 'none' : 'transfer'))}
          >
            Перемещение
          </button>
        </div>
      </div>

      {mode === 'movement' && (
        <div className="flex flex-col gap-3" data-testid="movement-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Причина
              <select
                className="input"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                aria-label="Причина"
              >
                {REASONS.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              {OUTBOUND.has(reason) ? 'Сколько списать' : 'Сколько добавить'}
              <input
                className="input"
                value={qty}
                onChange={(e) => setQty(e.target.value)}
                aria-label="Количество"
                inputMode="decimal"
              />
            </label>
          </div>
          <p className="muted text-[11.5px]">
            {reason === 'adjustment'
              ? 'Корректировка — единственная причина, которая может увести остаток в минус. Это механизм исправления ошибок.'
              : 'Указывается изменение остатка, а не итоговое количество.'}
          </p>
          <NoteField value={note} onChange={setNote} />
          {error && (
            <p className="text-[12px]" role="alert" data-testid="movement-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={post.isPending || !qty.trim()}
              onClick={() => void submitMovement()}
              data-testid="save-movement"
            >
              Провести
            </button>
          </div>
        </div>
      )}

      {mode === 'transfer' && (
        <div className="flex flex-col gap-3" data-testid="transfer-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Куда
              <select
                className="input"
                value={toLocation}
                onChange={(e) => setToLocation(e.target.value)}
                aria-label="Куда"
              >
                <option value="">— выберите —</option>
                {(locations.data ?? [])
                  .filter((l) => l.id !== locationId)
                  .map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.code} · {l.name}
                    </option>
                  ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Количество
              <input
                className="input"
                value={qty}
                onChange={(e) => setQty(e.target.value)}
                aria-label="Количество перемещения"
                inputMode="decimal"
              />
            </label>
          </div>
          <p className="muted text-[11.5px]">
            Перемещение записывается двумя строками с общим ref_id, сумма которых равна нулю.
          </p>
          <NoteField value={note} onChange={setNote} />
          {error && (
            <p className="text-[12px]" role="alert" data-testid="transfer-error">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={transfer.isPending || !toLocation || !qty.trim()}
              onClick={() => void submitTransfer()}
              data-testid="save-transfer"
            >
              Переместить
            </button>
          </div>
        </div>
      )}
    </section>
  );
}

function NoteField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <label className="flex flex-col gap-1 text-[12px] muted">
      Примечание
      <textarea
        className="input"
        rows={2}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Примечание"
      />
    </label>
  );
}

function LedgerTable({ rows }: { rows: StockMovementRow[] }) {
  const columns: RelatedColumn<StockMovementRow>[] = [
    {
      key: 'when',
      header: 'Когда',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.occurred_at)}</span>,
    },
    { key: 'reason', header: 'Причина', render: (r) => <StatusTag status={r.reason} /> },
    {
      key: 'delta',
      header: 'Изменение',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.qty_delta}</span>,
    },
    {
      key: 'balance',
      header: 'Остаток после',
      numeric: true,
      render: (r) => <span className="tabular-nums">{r.running_balance}</span>,
    },
    { key: 'who', header: 'Кто', render: (r) => orTBC(r.created_by) },
    { key: 'note', header: 'Примечание', render: (r) => orTBC(r.note) },
  ];

  return (
    <RelatedTable<StockMovementRow>
      title="Движения"
      columns={columns}
      rows={rows}
      rowKey={(r) => r.id}
      emptyLabel="Движений по этой позиции нет."
    />
  );
}
