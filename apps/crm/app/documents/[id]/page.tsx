'use client';

import { useParams } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { StatusTag } from '@/components/StatusTag';
import { WorkflowActions } from '@/components/WorkflowActions';
import { useDocument } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import type { Document } from '@samari/types';

/**
 * Документы — controlled document detail.
 *
 * The ladder is draft → approval → active, and only activation needs
 * `documents:approve`. `expiring` and `expired` are deliberately absent from the
 * buttons: they are not decisions anybody makes, they are conditions of a date
 * passing, derived by the alerts service.
 */

const DOC_LABELS: Record<string, string> = {
  draft: 'В черновик',
  approval: 'На согласование',
  active: 'Ввести в действие',
  superseded: 'Заменить',
  archived: 'В архив',
};

export default function DocumentDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'documents', 'manage');
  const doc = useDocument(params.id);
  const detail = doc.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.documents')}
        moduleHref="/documents"
        isLoading={doc.isLoading}
        error={
          doc.isError
            ? { status: doc.error instanceof ApiError ? doc.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.documents')}
            moduleHref="/documents"
            recordLabel={detail.doc_no}
            title={detail.title}
            identifier={detail.doc_no}
            status={<StatusTag status={detail.status} />}
            actions={
              <WorkflowActions
                endpoint={`/api/documents/${detail.id}/transition`}
                invalidate={['documents']}
                allowed={detail.allowed_transitions}
                labels={DOC_LABELS}
                disabled={!mayManage}
              />
            }
            groups={groupsFor(detail)}
            activity={<ActivityPanel resource="documents" resourceId={detail.id} />}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(doc: Document): FieldGroup[] {
  return [
    {
      title: 'Документ',
      fields: [
        { label: 'Номер', value: doc.doc_no },
        { label: 'Название', value: doc.title },
        { label: 'Тип', value: orTBC(doc.doc_type) },
        { label: 'Ответственный', value: orTBC(doc.owner_name) },
        { label: 'Действует до', value: orTBC(doc.valid_until) },
      ],
    },
  ];
}
