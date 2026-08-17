'use client';

import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';
import { ItemForm, emptyValues, toPayload } from '@/components/ItemForm';
import { useCreateItem } from '@/lib/items';
import { useSession, can } from '@/lib/session';

export default function NewItemPage() {
  const t = useTranslations();
  const router = useRouter();
  const session = useSession();
  const create = useCreateItem();

  if (session.isLoading) {
    return (
      <AppShell>
        <div className="muted py-16 text-center" role="status">
          Загрузка…
        </div>
      </AppShell>
    );
  }

  if (!can(session.data?.permissions, 'items', 'manage')) {
    return (
      <AppShell>
        <div className="card p-6 max-w-lg" role="alert">
          <h1 className="text-[17px] mb-2" style={{ fontFamily: 'var(--font-heading)' }}>
            Недостаточно прав
          </h1>
          <p className="muted text-[13px]">
            Для создания товаров требуется разрешение «Товары и цены: управление».
          </p>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell>
      <div className="mb-5">
        <div className="text-[11px] uppercase tracking-[0.18em] muted">{t('mod.items')}</div>
        <h1 className="text-[27px] leading-tight mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
          {t('newRecord')}
        </h1>
      </div>

      <ItemForm
        initial={emptyValues()}
        isSubmitting={create.isPending}
        error={create.error}
        submitLabel={t('save')}
        onCancel={() => router.push('/items')}
        onSubmit={(values) =>
          create.mutate(toPayload(values), {
            onSuccess: (item) => router.push(`/items/${item.id}`),
          })
        }
      />
    </AppShell>
  );
}
