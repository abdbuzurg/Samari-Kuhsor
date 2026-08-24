'use client';

import { CreateForm } from '@/components/CreateForm';
import { useCreateDocument } from '@/lib/operations';

/**
 * Новый документ.
 *
 * Created as a draft. `active` needs `documents:approve` and is reached through
 * the ladder on the document's own screen, never chosen here — a status
 * dropdown on a create form would bypass the approval it exists to enforce.
 */
export default function NewDocumentPage() {
  const create = useCreateDocument();

  return (
    <CreateForm
      moduleLabel="Документы"
      moduleHref="/documents"
      title="Новый документ"
      fields={[
        { name: 'doc_no', label: 'Номер', required: true, placeholder: 'DOC-001' },
        { name: 'title', label: 'Название', required: true },
        {
          name: 'doc_type',
          label: 'Тип',
          type: 'select',
          options: [
            { value: 'certificate', label: 'Сертификат' },
            { value: 'sop', label: 'Регламент' },
            { value: 'contract', label: 'Договор' },
            { value: 'manual', label: 'Руководство' },
            { value: 'permit', label: 'Разрешение' },
          ],
        },
        { name: 'valid_until', label: 'Действует до', type: 'date' },
      ]}
      onSubmit={async (v) => {
        const created = (await create.mutateAsync({
          doc_no: v.doc_no.trim(),
          title: v.title.trim(),
          doc_type: v.doc_type || undefined,
          valid_until: v.valid_until || undefined,
          version: 0,
        })) as { id?: string };
        return created?.id ?? null;
      }}
    />
  );
}
