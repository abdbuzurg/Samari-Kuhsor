'use client';

import { useParams, useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';
import { ItemForm, valuesFromItem, toPayload } from '@/components/ItemForm';
import { useItem, useUpdateItem } from '@/lib/items';
import { useSession, can } from '@/lib/session';

/**
 * Edit is a separate route, not the prototype's generic modal — that modal was a
 * placeholder (docs/05-MODULES.md:55).
 */
export default function EditItemPage() {
  const t = useTranslations();
  const router = useRouter();
  const params = useParams<{ id: string }>();

  const session = useSession();
  const item = useItem(params.id);
  const update = useUpdateItem(params.id);

  const mayManage = can(session.data?.permissions, 'items', 'manage');

  if (item.isLoading || session.isLoading) {
    return (
      <AppShell>
        <div className="muted py-16 text-center" role="status">
          Загрузка…
        </div>
      </AppShell>
    );
  }

  // Hiding the edit button is cosmetic; a user who navigates here directly must
  // still be refused (docs/04-RBAC.md:120). The server would refuse the PATCH
  // regardless — this just avoids showing a form that cannot be saved.
  if (!mayManage) {
    return (
      <AppShell>
        <div className="card p-6 max-w-lg" role="alert">
          <h1 className="text-[17px] mb-2" style={{ fontFamily: 'var(--font-heading)' }}>
            Недостаточно прав
          </h1>
          <p className="muted text-[13px]">
            Для редактирования товаров требуется разрешение «Товары и цены: управление».
          </p>
        </div>
      </AppShell>
    );
  }

  if (item.isError || !item.data) {
    return (
      <AppShell>
        <div className="card p-6 max-w-lg" role="alert">
          Товар не найден.
        </div>
      </AppShell>
    );
  }

  const it = item.data;

  return (
    <AppShell>
      <div className="mb-5">
        <div className="text-[11px] uppercase tracking-[0.18em] muted">{t('mod.items')}</div>
        <h1 className="text-[27px] leading-tight mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
          {it.sku}
        </h1>
      </div>

      <ItemForm
        initial={valuesFromItem(it)}
        // The version read with this record. The server rejects a stale one with
        // 409 rather than overwriting a colleague's edit.
        version={it.version}
        isSubmitting={update.isPending}
        error={update.error}
        submitLabel={t('save')}
        onCancel={() => router.push(`/items/${it.id}`)}
        onSubmit={(values) =>
          update.mutate(toPayload(values, it.version), {
            onSuccess: () => router.push(`/items/${it.id}`),
          })
        }
      />
    </AppShell>
  );
}
