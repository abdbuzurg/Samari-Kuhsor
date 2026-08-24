'use client';

import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { ActivityPanel } from '@/components/ActivityPanel';
import { AppShell } from '@/components/AppShell';
import { DetailShell } from '@/components/DetailShell';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { StatusTag } from '@/components/StatusTag';
import { useConvertInquiry, useInquiry } from '@/lib/operations';
import { orTBC } from '@/lib/resource';
import { useSession, can, ApiError } from '@/lib/session';
import { formatDateTime } from '@/lib/format';
import type { Inquiry } from '@samari/types';

/**
 * Интеграция с сайтом — enquiry detail.
 *
 * ToR §8 acceptance condition 1: website inquiries create CRM leads. The
 * endpoint and `useConvertInquiry` both existed; nothing called the hook, so
 * there was no way to convert anything.
 *
 * The reference number leads, because it is what the visitor holds and what they
 * quote on the phone.
 */
export default function InquiryDetailPage() {
  const t = useTranslations();
  const params = useParams<{ id: string }>();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'inquiries', 'manage');
  const inquiry = useInquiry(params.id);
  const detail = inquiry.data;

  return (
    <AppShell>
      <DetailShell
        moduleLabel={t('mod.inquiries')}
        moduleHref="/inquiries"
        isLoading={inquiry.isLoading}
        error={
          inquiry.isError
            ? { status: inquiry.error instanceof ApiError ? inquiry.error.status : undefined }
            : null
        }
      >
        {detail && (
          <DetailView
            moduleLabel={t('mod.inquiries')}
            moduleHref="/inquiries"
            recordLabel={detail.reference_no}
            title={detail.company?.trim() ? detail.company : detail.name}
            identifier={detail.reference_no}
            status={<StatusTag status={detail.status} />}
            actions={<ConvertAction inquiry={detail} mayManage={mayManage} />}
            groups={groupsFor(detail)}
            activity={<ActivityPanel resource="inquiries" resourceId={detail.id} />}
            footer={{
              createdAt: formatDateTime(detail.submitted_at),
              updatedAt: formatDateTime(detail.submitted_at),
              version: detail.version,
            }}
          />
        )}
      </DetailShell>
    </AppShell>
  );
}

function groupsFor(inq: Inquiry): FieldGroup[] {
  const groups: FieldGroup[] = [
    {
      title: 'Обращение',
      fields: [
        { label: 'Номер обращения', value: inq.reference_no },
        { label: 'Тип', value: <StatusTag status={inq.type} /> },
        { label: 'Имя', value: inq.name },
        { label: 'Компания', value: orTBC(inq.company) },
        { label: 'Контакт', value: inq.contact },
        { label: 'Получено', value: formatDateTime(inq.submitted_at) },
        { label: 'Сообщение', value: orTBC(inq.message), wide: true },
      ],
    },
  ];

  // A complaint must name a batch — the domain refuses one without. That link is
  // the entry point to the ToR's complaint → traceability → investigation
  // workflow, so it gets its own band rather than being a field among many.
  if (inq.type.key === 'complaint') {
    groups.push({
      title: 'Прослеживаемость',
      fields: [
        {
          label: 'Партия с упаковки',
          value: inq.batch_id ? (
            <Link href={`/quality/${inq.batch_id}`} className="hover:underline">
              {inq.batch_no}
            </Link>
          ) : (
            orTBC(inq.batch_no)
          ),
        },
      ],
    });
  }

  return groups;
}

/**
 * Conversion.
 *
 * Creates a customer and a lead, carrying the reference number across. Converting
 * twice would put two leads behind one enquiry, so the domain refuses it and the
 * button disappears once the status has moved on.
 */
function ConvertAction({ inquiry, mayManage }: { inquiry: Inquiry; mayManage: boolean }) {
  const convert = useConvertInquiry(inquiry.id);
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);

  if (!mayManage || inquiry.status.key !== 'new') return null;

  return (
    <div className="flex flex-col items-end gap-1">
      <button
        type="button"
        className="btn btn-primary"
        disabled={convert.isPending}
        data-testid="convert-inquiry"
        onClick={async () => {
          setError(null);
          try {
            const lead = (await convert.mutateAsync(undefined)) as { customer_id?: string };
            // Land on the customer the conversion just created, so the enquiry
            // leads somewhere rather than merely changing colour.
            // The customer route is /crm/{id}. An earlier draft pushed
            // /crm/customers/{id}, which does not exist — conversion succeeded and
            // landed on a 404.
            if (lead?.customer_id) router.push(`/crm/${lead.customer_id}`);
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Не удалось создать лид');
          }
        }}
      >
        Создать лид
      </button>
      {error && (
        <span className="text-[12px]" role="alert" data-testid="convert-error">
          {error}
        </span>
      )}
    </div>
  );
}
