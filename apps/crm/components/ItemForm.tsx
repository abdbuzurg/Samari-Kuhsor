'use client';

import { useState, type FormEvent } from 'react';

import { ApiError } from '@/lib/session';
import type { Item } from '@samari/types';

/**
 * Create/edit form for Товары.
 *
 * Two things this form does that every later module's form must copy:
 *
 *   1. It carries the `version` it read and surfaces a 409 as a CONFLICT the
 *      user can act on, never as a generic failure and never by silently
 *      retrying. Retrying a version conflict is how you overwrite the edit the
 *      guard just protected.
 *   2. It renders per-field errors from `error.details`, keyed on the stable
 *      field codes the API returns, so "SKU уже используется" appears against
 *      the SKU input rather than in a banner.
 *
 * Status is a field here because for `items` it IS the publication state
 * (docs/02-SCHEMA.md:141) — an ordinary attribute, not a workflow transition.
 * Modules whose status is a decision (a batch release, a PO approval) must use an
 * action button instead: docs/05-MODULES.md §2 forbids a status dropdown there.
 */

export interface ItemFormValues {
  sku: string;
  item_type: string;
  category: string;
  base_uom: string;
  status: string;
  shelf_life_days: string;
  min_qty: string;
  names: { ru: string; tg: string; en: string };
}

export function emptyValues(): ItemFormValues {
  return {
    sku: '',
    item_type: 'finished_good',
    category: '',
    base_uom: 'bottle',
    // A new product starts as a draft. `status = active` publishes a finished
    // good to the public website (docs/02-SCHEMA.md:141), and that must be a
    // deliberate act, not the default.
    status: 'draft',
    shelf_life_days: '',
    min_qty: '',
    names: { ru: '', tg: '', en: '' },
  };
}

export function valuesFromItem(item: Item): ItemFormValues {
  return {
    sku: item.sku,
    item_type: item.item_type,
    category: item.category ?? '',
    base_uom: item.base_uom,
    status: item.status.key,
    shelf_life_days: item.shelf_life_days == null ? '' : String(item.shelf_life_days),
    min_qty: item.min_qty ?? '',
    names: {
      ru: item.translations.ru?.name ?? '',
      tg: item.translations.tg?.name ?? '',
      en: item.translations.en?.name ?? '',
    },
  };
}

/** Builds the request body. Empty optional fields become null, never "" —
 *  «уточняется» depends on null (docs/02-SCHEMA.md:176). */
export function toPayload(v: ItemFormValues, version?: number): Record<string, unknown> {
  const translations: Record<string, { name: string }> = {};
  for (const [locale, name] of Object.entries(v.names)) {
    if (name.trim()) translations[locale] = { name: name.trim() };
  }

  const body: Record<string, unknown> = {
    category: v.category.trim() || null,
    base_uom: v.base_uom.trim(),
    status: v.status,
    shelf_life_days: v.shelf_life_days.trim() ? Number(v.shelf_life_days) : null,
    // Quantities stay strings the whole way to the server (03-API-CONTRACT:147).
    min_qty: v.min_qty.trim() || null,
    translations,
  };

  if (version === undefined) {
    body.sku = v.sku.trim();
    body.item_type = v.item_type;
    body.packaging_units = [{ code: baseUnitCode(v.base_uom), qty_in_base: '1.000' }];
  } else {
    // SKU and item_type are absent on update: changing either once batches and
    // stock movements reference the item would rewrite history other records
    // point at. The server ignores them; not sending them makes that explicit.
    body.version = version;
  }
  return body;
}

function baseUnitCode(uom: string): string {
  return uom === 'jar' ? 'JAR' : 'BOTTLE';
}

const ITEM_TYPES = [
  { value: 'finished_good', label: 'Готовая продукция' },
  { value: 'raw_material', label: 'Сырьё' },
  { value: 'packaging', label: 'Упаковка' },
];

const STATUSES = [
  { value: 'draft', label: 'Черновик' },
  { value: 'active', label: 'Активен' },
  { value: 'archived', label: 'Архив' },
];

const UOMS = ['bottle', 'jar', 'kg', 'l', 'pcs'];

export interface ItemFormProps {
  initial: ItemFormValues;
  /** Present when editing; absent when creating. */
  version?: number;
  onSubmit: (values: ItemFormValues) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  error: unknown;
  submitLabel: string;
}

export function ItemForm({
  initial,
  version,
  onSubmit,
  onCancel,
  isSubmitting,
  error,
  submitLabel,
}: ItemFormProps) {
  const [values, setValues] = useState(initial);
  const editing = version !== undefined;

  const apiError = error instanceof ApiError ? error : null;
  const conflict = apiError?.code === 'version_conflict';
  const fieldError = (field: string) => apiError?.forField(field)?.message;

  const set = <K extends keyof ItemFormValues>(key: K, value: ItemFormValues[K]) =>
    setValues((v) => ({ ...v, [key]: value }));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit(values);
  };

  return (
    <form onSubmit={submit} noValidate data-testid="item-form">
      {conflict && (
        // A version conflict is not a validation error and must not be retried
        // automatically: someone else's change is already saved, and the only
        // safe move is to show it and let this user decide.
        <div className="card p-4 mb-4" role="alert" data-testid="version-conflict">
          <div className="tag tag-warn mb-2">Конфликт версий</div>
          <p className="text-[13px]">
            Запись была изменена другим пользователем, пока вы её редактировали. Обновите страницу,
            чтобы увидеть актуальные данные — ваши изменения не сохранены.
          </p>
          <button
            type="button"
            className="btn btn-secondary mt-3"
            onClick={() => window.location.reload()}
          >
            Обновить
          </button>
        </div>
      )}

      {apiError && !conflict && !apiError.details?.length && (
        <div className="tag tag-danger mb-4 w-full justify-center py-2" role="alert">
          {apiError.message || 'Не удалось сохранить запись'}
        </div>
      )}

      <section className="card p-5 mb-4">
        <h2 className="text-[15px] mb-4" style={{ fontFamily: 'var(--font-heading)' }}>
          Идентификация
        </h2>
        <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(2, minmax(0, 1fr))' }}>
          <Field label="SKU" error={fieldError('sku')} required>
            <input
              className="input w-full"
              value={values.sku}
              // Immutable once created: batches and stock movements reference it.
              disabled={editing}
              onChange={(e) => set('sku', e.target.value)}
            />
            {editing && (
              <p className="text-[11px] muted mt-1">
                Артикул нельзя изменить: на него ссылаются партии и движения склада.
              </p>
            )}
          </Field>

          <Field label="Тип позиции" error={fieldError('item_type')} required>
            <select
              className="input w-full"
              value={values.item_type}
              disabled={editing}
              onChange={(e) => set('item_type', e.target.value)}
            >
              {ITEM_TYPES.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            {!editing && values.item_type !== 'finished_good' && (
              // D8: raw materials are RAW-, packaging is PKG-. Saying so before
              // submission is kinder than a 400 afterwards.
              <p className="text-[11px] muted mt-1">
                Артикул должен начинаться с {values.item_type === 'raw_material' ? 'RAW-' : 'PKG-'}
              </p>
            )}
          </Field>

          <Field label="Категория" error={fieldError('category')}>
            <input
              className="input w-full"
              value={values.category}
              onChange={(e) => set('category', e.target.value)}
            />
          </Field>

          <Field label="Базовая единица" error={fieldError('base_uom')} required>
            <select
              className="input w-full"
              value={values.base_uom}
              onChange={(e) => set('base_uom', e.target.value)}
            >
              {UOMS.map((u) => (
                <option key={u} value={u}>
                  {u}
                </option>
              ))}
            </select>
          </Field>

          <Field label="Статус" error={fieldError('status')} required>
            <select
              className="input w-full"
              value={values.status}
              onChange={(e) => set('status', e.target.value)}
            >
              {STATUSES.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            {values.item_type === 'finished_good' && (
              <p className="text-[11px] muted mt-1">
                «Активен» публикует товар на сайте.
              </p>
            )}
          </Field>

          <Field label="Минимальный остаток" error={fieldError('min_qty')}>
            <input
              className="input w-full"
              inputMode="decimal"
              value={values.min_qty}
              placeholder="не задан"
              onChange={(e) => set('min_qty', e.target.value)}
            />
            <p className="text-[11px] muted mt-1">Порог для оповещения о низком остатке.</p>
          </Field>
        </div>
      </section>

      <section className="card p-5 mb-4">
        <h2 className="text-[15px] mb-1" style={{ fontFamily: 'var(--font-heading)' }}>
          Наименования
        </h2>
        <p className="text-[12px] muted mb-4">
          Достаточно заполнить хотя бы один язык. Незаполненные подставляются из русского.
        </p>
        <div className="grid gap-4">
          {(
            [
              ['ru', 'Русский'],
              ['tg', 'Тоҷикӣ'],
              ['en', 'English'],
            ] as const
          ).map(([code, label]) => (
            <Field key={code} label={label} error={fieldError(`translations.${code}.name`)}>
              <input
                className="input w-full"
                value={values.names[code]}
                onChange={(e) => set('names', { ...values.names, [code]: e.target.value })}
              />
            </Field>
          ))}
        </div>
      </section>

      <section className="card p-5 mb-4">
        <h2 className="text-[15px] mb-1" style={{ fontFamily: 'var(--font-heading)' }}>
          Срок годности
        </h2>
        {/* Composition, nutrition and storage are deliberately NOT editable here.
            They stay null and render «уточняется» until the client's recipes are
            approved and lab-verified — the client's own rule that the system must
            not publish unverified claims (docs/02-SCHEMA.md:176). Offering the
            fields would invite someone to fill them in with a plausible guess. */}
        <p className="text-[12px] muted mb-4">
          Состав, пищевая ценность и условия хранения публикуются только после утверждения рецептур
          и лабораторной проверки.
        </p>
        <Field label="Срок годности, дней" error={fieldError('shelf_life_days')}>
          <input
            className="input w-full max-w-[200px]"
            inputMode="numeric"
            value={values.shelf_life_days}
            placeholder="уточняется"
            onChange={(e) => set('shelf_life_days', e.target.value)}
          />
        </Field>
      </section>

      <div className="flex items-center gap-2">
        <button
          type="submit"
          className="btn"
          style={{ background: 'var(--color-accent)', color: 'var(--color-bg)' }}
          disabled={isSubmitting || conflict}
        >
          {isSubmitting ? 'Сохранение…' : submitLabel}
        </button>
        <button type="button" className="btn btn-secondary" onClick={onCancel}>
          Отмена
        </button>
      </div>
    </form>
  );
}

function Field({
  label,
  error,
  required,
  children,
}: {
  label: string;
  error?: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="block text-[12px] mb-1">
        {label}
        {required && <span aria-hidden> *</span>}
      </span>
      {children}
      {error && (
        <span className="block text-[11px] mt-1" style={{ color: 'var(--sk-danger-t)' }} role="alert">
          {error}
        </span>
      )}
    </label>
  );
}
