'use client';

import { useParams } from 'next/navigation';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { RelatedTable, type RelatedColumn } from '@/components/RelatedTable';
import { StatusTag } from '@/components/StatusTag';
import { WorkflowActions } from '@/components/WorkflowActions';
import { useDeal } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { Deal, DealStageEvent } from '@samari/types';

/**
 * Сделка — deal detail with its stage history.
 *
 * The history is `deal_stage_events`: immutable evidence with no version and no
 * deleted_at, exactly like `batch_status_events`. A pipeline whose history can be
 * rewritten is not a pipeline.
 */

const STAGE_LABELS: Record<string, string> = {
  new: 'В новые лиды',
  negotiation: 'В переговоры',
  quoted: 'КП отправлено',
  won: 'Выиграно',
  lost: 'Проиграно',
};

export default function DealDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'crm', 'manage');
  const deal = useDeal(params.id);
  const detail = deal.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.crm')}
        moduleHref="/crm/pipeline"
        isLoading={deal.isLoading}
        error={
          deal.isError
            ? { status: deal.error instanceof ApiError ? deal.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel="Воронка сделок"
            moduleHref="/crm/pipeline"
            recordLabel={detail.customer_name}
            title={detail.customer_name}
            identifier={detail.amount ? `${detail.amount} с.` : undefined}
            status={<StatusTag status={detail.stage} />}
            actions={
              <WorkflowActions
                endpoint={`/api/deals/${detail.id}/stage`}
                invalidate={['deals', 'customer-detail', 'crm-kpis']}
                allowed={detail.allowed_transitions}
                labels={STAGE_LABELS}
                disabled={!mayManage}
                // Won and lost are terminal; the server returns an empty list for
                // them, so no button appears rather than one that fails.
                reasonFor={(to) => to === 'lost'}
              />
            }
            groups={groupsFor(detail)}
            related={<StageHistory history={detail.history} />}
            activity={<ActivityPanel resource="crm" resourceId={detail.id} />}
            footer={{
              createdAt: formatDateTime(detail.created_at),
              updatedAt: formatDateTime(detail.created_at),
              version: detail.version,
            }}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(deal: Deal): FieldGroup[] {
  return [
    {
      title: 'Сделка',
      fields: [
        {
          label: 'Клиент',
          value: (
            <Link href={`/crm/${deal.customer_id}`} className="hover:underline">
              {deal.customer_name}
            </Link>
          ),
        },
        { label: 'Сумма', value: deal.amount ? `${deal.amount} с.` : 'уточняется' },
        { label: 'Менеджер', value: orTBC(deal.owner_name) },
        { label: 'Ожидаемое закрытие', value: orTBC(deal.expected_close) },
      ],
    },
  ];
}

function StageHistory({ history }: { history: DealStageEvent[] }) {
  const columns: RelatedColumn<DealStageEvent>[] = [
    {
      key: 'move',
      header: 'Переход',
      render: (r) => (
        <span className="flex items-center gap-1.5 flex-wrap">
          {r.from_stage ? <StatusTag status={r.from_stage} /> : <span className="muted">—</span>}
          <span className="muted">→</span>
          <StatusTag status={r.to_stage} />
        </span>
      ),
    },
    { key: 'who', header: 'Кто', render: (r) => orTBC(r.changed_by) },
    { key: 'note', header: 'Комментарий', render: (r) => orTBC(r.note) },
    {
      key: 'when',
      header: 'Когда',
      render: (r) => <span className="tabular-nums">{formatDateTime(r.occurred_at)}</span>,
    },
  ];

  return (
    <RelatedTable<DealStageEvent>
      title="История стадий"
      columns={columns}
      rows={history}
      rowKey={(r) => r.id}
      emptyLabel="Стадии не менялись."
    />
  );
}
