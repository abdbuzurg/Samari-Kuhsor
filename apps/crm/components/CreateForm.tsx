'use client';

import { useRouter } from 'next/navigation';
import { useState, type ReactNode } from 'react';

import { AppShell } from '@/components/AppShell';

/**
 * The shared create-record scaffold.
 *
 * Ten modules needed a create form. Without this each would re-invent the same
 * four things — a field grid, a validation error surface, a cancel that goes
 * back to the register, and a save that lands on the record it just made.
 *
 * Errors are the SERVER's own text. A field-level validation failure names the
 * field and the rule; a generic "не удалось сохранить" throws that away and
 * leaves the user guessing which input was wrong.
 */

export interface FormField {
  name: string;
  label: string;
  type?: 'text' | 'date' | 'number' | 'select' | 'textarea';
  options?: Array<{ value: string; label: string }>;
  required?: boolean;
  placeholder?: string;
  /** Span the whole row. */
  wide?: boolean;
}

export function CreateForm({
  moduleLabel,
  moduleHref,
  title,
  fields,
  columns = 2,
  submitLabel = 'Сохранить',
  onSubmit,
  extra,
}: {
  moduleLabel: string;
  moduleHref: string;
  title: string;
  fields: FormField[];
  columns?: number;
  submitLabel?: string;
  /** Returns the created record's id, or null to stay on the register. */
  onSubmit: (values: Record<string, string>) => Promise<string | null>;
  /** Anything the plain field grid cannot express — order lines, for instance. */
  extra?: (values: Record<string, string>) => ReactNode;
}) {
  const router = useRouter();
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(fields.map((f) => [f.name, f.options?.[0]?.value ?? ''])),
  );
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const missing = fields.filter((f) => f.required && !values[f.name]?.trim());

  function set(name: string, value: string) {
    setValues((v) => ({ ...v, [name]: value }));
  }

  return (
    <AppShell>
      <nav className="text-[12px] muted mb-3">
        <a href={moduleHref} className="hover:underline">
          {moduleLabel}
        </a>
      </nav>
      <h1 className="text-[27px] leading-[1.05] mb-4" style={{ fontFamily: 'var(--font-heading)' }}>
        {title}
      </h1>

      <section className="card p-5 flex flex-col gap-3 max-w-3xl" data-testid="create-form">
        <div
          className={`grid gap-3 grid-cols-1 ${columns === 3 ? 'sm:grid-cols-3' : 'sm:grid-cols-2'}`}
        >
          {fields.map((field) => (
            <label
              key={field.name}
              className="flex flex-col gap-1 text-[12px] muted"
              style={field.wide ? { gridColumn: '1 / -1' } : undefined}
            >
              {field.label}
              {field.type === 'select' ? (
                <select
                  className="input"
                  aria-label={field.label}
                  value={values[field.name] ?? ''}
                  onChange={(e) => set(field.name, e.target.value)}
                >
                  {!field.required && <option value="">— не выбрано —</option>}
                  {(field.options ?? []).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              ) : field.type === 'textarea' ? (
                <textarea
                  className="input"
                  rows={3}
                  aria-label={field.label}
                  value={values[field.name] ?? ''}
                  onChange={(e) => set(field.name, e.target.value)}
                />
              ) : (
                <input
                  className="input"
                  type={field.type === 'date' ? 'date' : 'text'}
                  inputMode={field.type === 'number' ? 'decimal' : undefined}
                  aria-label={field.label}
                  placeholder={field.placeholder}
                  value={values[field.name] ?? ''}
                  onChange={(e) => set(field.name, e.target.value)}
                />
              )}
            </label>
          ))}
        </div>

        {extra?.(values)}

        {error && (
          <p className="text-[12px]" role="alert" data-testid="create-error">
            {error}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-secondary" onClick={() => router.push(moduleHref)}>
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={saving || missing.length > 0}
            data-testid="create-save"
            onClick={async () => {
              setError(null);
              setSaving(true);
              try {
                const id = await onSubmit(values);
                router.push(id ? `${moduleHref}/${id}` : moduleHref);
              } catch (e) {
                // The server's own message names the field and the rule.
                setError(e instanceof Error ? e.message : 'Не удалось сохранить');
              } finally {
                setSaving(false);
              }
            }}
          >
            {submitLabel}
          </button>
        </div>
      </section>
    </AppShell>
  );
}
