'use client';

import { useParams } from 'next/navigation';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import { useAsset, useAssetMaintenance, useRecordMaintenance } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { Asset, MaintenanceEvent } from '@samari/types';

/**
 * Оборудование и ТО — asset detail.
 *
 * Recording a service clears the `maintenance_due` flag. It deliberately does NOT
 * clear `broken`: whether a repair actually worked is a judgement someone makes,
 * not a consequence of filing paperwork about it.
 */
export default function AssetDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'equipment', 'manage');
  const asset = useAsset(params.id);
  const history = useAssetMaintenance(params.id);
  const detail = asset.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.equipment')}
        moduleHref="/equipment"
        isLoading={asset.isLoading}
        error={
          asset.isError
            ? { status: asset.error instanceof ApiError ? asset.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.equipment')}
            moduleHref="/equipment"
            recordLabel={detail.asset_no}
            title={detail.name}
            identifier={detail.asset_no}
            status={<StatusTag status={detail.status} />}
            groups={groupsFor(detail)}
            related={
              <MaintenanceLog
                id={detail.id}
                events={history.data ?? []}
                mayManage={mayManage}
              />
            }
            activity={<ActivityPanel resource="equipment" resourceId={detail.id} />}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(asset: Asset): FieldGroup[] {
  return [
    {
      title: 'Оборудование',
      fields: [
        { label: 'Инв. номер', value: asset.asset_no },
        { label: 'Наименование', value: asset.name },
        { label: 'Тип', value: orTBC(asset.asset_type) },
        { label: 'Линия', value: orTBC(asset.line) },
        { label: 'Введено в эксплуатацию', value: orTBC(asset.commissioned_on) },
        { label: 'Гарантия до', value: orTBC(asset.warranty_until) },
        { label: 'Следующее ТО', value: orTBC(asset.next_due_on) },
        { label: 'Последнее обслуживание', value: formatDateTime(asset.last_service_at) },
      ],
    },
  ];
}

function MaintenanceLog({
  id,
  events,
  mayManage,
}: {
  id: string;
  events: MaintenanceEvent[];
  mayManage: boolean;
}) {
  const record = useRecordMaintenance(id);
  const [open, setOpen] = useState(false);
  const [eventType, setEventType] = useState('planned');
  const [performedAt, setPerformedAt] = useState('');
  const [nextDue, setNextDue] = useState('');
  const [notes, setNotes] = useState('');
  const [error, setError] = useState<string | null>(null);

  const columns: RelatedColumn<MaintenanceEvent>[] = [
    { key: 'type', header: 'Тип', render: (r) => orTBC(r.event_type) },
    { key: 'done', header: 'Выполнено', render: (r) => formatDateTime(r.performed_at) },
    { key: 'next', header: 'Следующее', render: (r) => orTBC(r.next_due_on) },
    { key: 'notes', header: 'Примечание', render: (r) => orTBC(r.notes) },
  ];

  async function submit() {
    setError(null);
    try {
      await record.mutateAsync({
        event_type: eventType,
        performed_at: performedAt.trim() || undefined,
        next_due_on: nextDue.trim() || undefined,
        notes: notes.trim() || undefined,
      });
      setOpen(false);
      setPerformedAt('');
      setNextDue('');
      setNotes('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось записать обслуживание');
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <RelatedTable<MaintenanceEvent>
        title="История обслуживания"
        columns={columns}
        rows={events}
        rowKey={(r) => r.id}
        emptyLabel="Обслуживание ещё не проводилось."
        action={
          mayManage ? (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => setOpen((v) => !v)}
              data-testid="toggle-maintenance-form"
            >
              {open ? 'Отмена' : 'Записать ТО'}
            </button>
          ) : undefined
        }
      />

      {open && (
        <section className="card p-4 flex flex-col gap-3" data-testid="maintenance-form">
          <div className="grid gap-3 grid-cols-1 sm:grid-cols-3">
            <label className="flex flex-col gap-1 text-[12px] muted">
              Тип
              <select
                className="input"
                value={eventType}
                onChange={(e) => setEventType(e.target.value)}
                aria-label="Тип"
              >
                <option value="planned">Плановое</option>
                <option value="breakdown">Поломка</option>
                <option value="calibration">Калибровка</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Выполнено
              <input
                className="input"
                type="date"
                value={performedAt}
                onChange={(e) => setPerformedAt(e.target.value)}
                aria-label="Выполнено"
              />
            </label>
            <label className="flex flex-col gap-1 text-[12px] muted">
              Следующее ТО
              <input
                className="input"
                type="date"
                value={nextDue}
                onChange={(e) => setNextDue(e.target.value)}
                aria-label="Следующее ТО"
              />
            </label>
          </div>

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
            <p className="text-[12px]" role="alert" data-testid="maintenance-error">
              {error}
            </p>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              className="btn btn-primary"
              disabled={record.isPending}
              onClick={() => void submit()}
              data-testid="save-maintenance"
            >
              Сохранить
            </button>
          </div>
        </section>
      )}
    </div>
  );
}
