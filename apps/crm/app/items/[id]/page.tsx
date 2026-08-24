'use client';

import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useLocale, useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';
import { ActivityPanel } from '@/components/ActivityPanel';
import { DetailView, type FieldGroup } from '@/components/DetailView';
import { StatusTag } from '@/components/StatusTag';
import { useItem, orTBC } from '@/lib/items';
import { useSession, can } from '@/lib/session';

/** Товары и цены — detail view. Sections per docs/05-MODULES.md:88. */
export default function ItemDetailPage() {
  const t = useTranslations();
  const locale = useLocale();
  const params = useParams<{ id: string }>();

  const session = useSession();
  const mayManage = can(session.data?.permissions, 'items', 'manage');
  const item = useItem(params.id);

  if (item.isLoading) {
    return (
      <AppShell>
        <div className="muted py-16 text-center" role="status">
          Загрузка…
        </div>
      </AppShell>
    );
  }

  if (item.isError || !item.data) {
    return (
      <AppShell>
        <div className="card p-6 max-w-lg" role="alert">
          <h1 className="text-[17px] mb-2" style={{ fontFamily: 'var(--font-heading)' }}>
            Товар не найден
          </h1>
          <p className="muted text-[13px] mb-4">
            Запись могла быть удалена или у вас нет доступа к ней.
          </p>
          <Link href="/items" className="btn btn-secondary">
            К списку товаров
          </Link>
        </div>
      </AppShell>
    );
  }

  const it = item.data;
  const tr = it.translations[locale] ?? it.translations.ru;
  const name = tr?.name ?? it.sku;

  const groups: FieldGroup[] = [
    {
      title: 'Идентификация',
      fields: [
        { label: 'SKU', value: <span className="tabular-nums">{it.sku}</span> },
        { label: 'Тип позиции', value: it.item_type },
        { label: 'Категория', value: orTBC(it.category) },
        { label: 'Базовая единица', value: it.base_uom },
        {
          label: 'Минимальный остаток',
          // Drives the low-stock alert. Absent means no threshold is set, which
          // is different from a threshold of zero.
          value: it.min_qty ? `${it.min_qty} ${it.base_uom}` : orTBC(null),
        },
      ],
    },
    {
      title: 'Наименования',
      fields: (['ru', 'tg', 'en'] as const).map((code) => ({
        label: { ru: 'Русский', tg: 'Тоҷикӣ', en: 'English' }[code],
        // A missing locale is «уточняется», not blank: the translations are an
        // outstanding client dependency (D10), and blank reads as "no name".
        value: orTBC(it.translations[code]?.name),
      })),
    },
    {
      title: 'Состав и хранение',
      fields: [
        // These stay null until the recipes are lab-verified. The client set the
        // rule that the system must not publish unverified claims
        // (docs/02-SCHEMA.md:176), so the placeholder is the correct output here,
        // not a gap to be filled in later with a guess.
        { label: 'Состав', value: orTBC(tr?.ingredients), wide: true },
        { label: 'Пищевая ценность', value: orTBC(tr?.nutrition), wide: true },
        { label: 'Условия хранения', value: orTBC(tr?.storage_conditions), wide: true },
        { label: 'После вскрытия', value: orTBC(tr?.after_opening), wide: true },
        {
          label: 'Срок годности',
          value: it.shelf_life_days ? `${it.shelf_life_days} дн` : orTBC(null),
        },
      ],
    },
  ];

  const related = (
    <>
      <section className="card p-5">
        <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
          Упаковка
        </h2>
        {it.packaging_units.length === 0 ? (
          <p className="muted text-[13px]">Упаковочные единицы не заданы.</p>
        ) : (
          <table className="table w-full">
            <thead>
              <tr>
                <th>Код</th>
                <th className="text-right">В базовых единицах</th>
                <th>Штрихкод</th>
              </tr>
            </thead>
            <tbody>
              {it.packaging_units.map((u) => (
                <tr key={u.code}>
                  <td>{u.code}</td>
                  <td className="text-right tabular-nums">{u.qty_in_base}</td>
                  {/* EAN-13 stays null until register question Q4 is answered
                      (docs/01-DECISIONS.md, open questions). */}
                  <td className="muted">{orTBC(u.barcode)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="card p-5">
        <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
          История цен
        </h2>
        {it.price_history.length === 0 ? (
          <p className="muted text-[13px]">Цена ещё не назначена.</p>
        ) : (
          <table className="table w-full">
            <thead>
              <tr>
                <th className="text-right">Цена</th>
                <th>Валюта</th>
                <th>Действует с</th>
                <th>по</th>
              </tr>
            </thead>
            <tbody>
              {it.price_history.map((p) => (
                <tr key={`${p.valid_from}-${p.amount}`}>
                  <td className="text-right tabular-nums">{p.amount}</td>
                  <td>{p.currency}</td>
                  <td className="tabular-nums">{p.valid_from}</td>
                  {/* An open price has no end date; it is current, not missing. */}
                  <td className="tabular-nums muted">{p.valid_to ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );

  return (
    <AppShell>
      <DetailView
        moduleLabel={t('mod.items')}
        moduleHref="/items"
        recordLabel={it.sku}
        title={name}
        identifier={it.sku}
        status={<StatusTag status={it.status} />}
        actions={
          mayManage ? (
            <Link href={`/items/${it.id}/edit`} className="btn btn-secondary">
              Редактировать
            </Link>
          ) : undefined
        }
        groups={groups}
        related={related}
        activity={<ActivityPanel resource="items" resourceId={it.id} />}
        footer={{
          createdAt: formatDateTime(it.created_at),
          updatedAt: formatDateTime(it.updated_at),
          version: it.version,
        }}
      />
    </AppShell>
  );
}

/**
 * Renders an RFC 3339 UTC timestamp in Dushanbe time.
 *
 * The API sends UTC and says so (docs/03-API-CONTRACT.md:145); presenting it in
 * local time is explicitly the frontend's job. Asia/Dushanbe is fixed rather than
 * read from the browser: a director opening the CRM from abroad should see the
 * factory's clock, because that is when the thing actually happened.
 */
function formatDateTime(iso: string): string {
  if (!iso) return '—';
  return new Intl.DateTimeFormat('ru-RU', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'Asia/Dushanbe',
  }).format(new Date(iso));
}
