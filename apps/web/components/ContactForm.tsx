'use client';

import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { palette } from '@/lib/palette';

/**
 * The enquiry form.
 *
 * Everything that matters happens in Go: validation, the batch requirement for
 * complaints, and the per-IP rate limit. This component's job is to send the
 * form and then tell the visitor the truth about what happened — in particular
 * to show them their reference number, which is the only receipt they get and
 * the thing QOIM will ask for when they call.
 *
 * The success state deliberately replaces the form rather than sitting above it.
 * A visitor who cannot see the fields cannot submit twice by accident, and
 * duplicate enquiries are the client's own complaint about their old process.
 */

type State =
  | { status: 'idle' }
  | { status: 'sending' }
  | { status: 'sent'; reference: string }
  | { status: 'error'; fields: Record<string, string> };

const TYPES = ['wholesale', 'distributor', 'contact', 'complaint', 'job'] as const;

export function ContactForm() {
  const t = useTranslations('contact');
  const [state, setState] = useState<State>({ status: 'idle' });

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setState({ status: 'sending' });

    try {
      const res = await fetch('/api/inquiries', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: String(form.get('type') ?? 'contact'),
          name: String(form.get('name') ?? ''),
          company: emptyToNull(form.get('company')),
          contact: String(form.get('contact') ?? ''),
          message: emptyToNull(form.get('message')),
        }),
      });
      const body = await res.json().catch(() => null);

      if (!res.ok) {
        // Field errors come back in the contract's `details` array; anything
        // else is shown as a single message rather than invented per-field.
        const fields: Record<string, string> = {};
        for (const d of body?.error?.details ?? []) {
          if (d?.field && d?.message) fields[d.field] = d.message;
        }
        setState({ status: 'error', fields });
        return;
      }
      setState({ status: 'sent', reference: body?.data?.reference_no ?? '' });
    } catch {
      setState({ status: 'error', fields: {} });
    }
  }

  if (state.status === 'sent') {
    return (
      <div
        role="status"
        data-testid="contact-sent"
        style={{
          background: '#fff',
          border: `1px solid ${palette.hairlineStrong}`,
          borderRadius: 16,
          padding: '28px 26px',
        }}
      >
        <h2 style={{ fontSize: 18, margin: '0 0 10px', fontWeight: 800 }}>{t('sentTitle')}</h2>
        <p style={{ fontSize: 14.5, lineHeight: 1.6, margin: 0, color: palette.muted }}>
          {t('sentBody', { reference: state.reference })}
        </p>
      </div>
    );
  }

  const fieldErrors = state.status === 'error' ? state.fields : {};
  const generalError = state.status === 'error' && Object.keys(fieldErrors).length === 0;

  return (
    <form
      onSubmit={onSubmit}
      noValidate
      style={{
        background: '#fff',
        border: `1px solid ${palette.hairline}`,
        borderRadius: 16,
        padding: '26px 24px 28px',
        display: 'grid',
        gap: 16,
      }}
    >
      <h2 style={{ fontSize: 16, margin: 0, fontWeight: 700 }}>{t('formTitle')}</h2>

      {generalError && (
        <p
          role="alert"
          data-testid="contact-error"
          style={{ margin: 0, fontSize: 13.5, color: '#B03A24' }}
        >
          {t('errorBody')}
        </p>
      )}

      <Field label={t('type')} htmlFor="type" error={fieldErrors.type}>
        <select id="type" name="type" defaultValue="wholesale" style={controlStyle}>
          {TYPES.map((key) => (
            <option key={key} value={key}>
              {t(
                `type${key.charAt(0).toUpperCase()}${key.slice(1)}` as
                  | 'typeWholesale'
                  | 'typeDistributor'
                  | 'typeContact'
                  | 'typeComplaint'
                  | 'typeJob',
              )}
            </option>
          ))}
        </select>
      </Field>

      <Field label={t('name')} htmlFor="name" error={fieldErrors.name} required>
        <input id="name" name="name" required style={controlStyle} autoComplete="name" />
      </Field>

      <Field label={t('company')} htmlFor="company" error={fieldErrors.company}>
        <input id="company" name="company" style={controlStyle} autoComplete="organization" />
      </Field>

      <Field label={t('contactField')} htmlFor="contact" error={fieldErrors.contact} required>
        <input id="contact" name="contact" required style={controlStyle} autoComplete="email tel" />
      </Field>

      <Field label={t('message')} htmlFor="message" error={fieldErrors.message}>
        <textarea id="message" name="message" rows={4} style={controlStyle} />
      </Field>

      <button
        type="submit"
        disabled={state.status === 'sending'}
        style={{
          fontSize: 15,
          fontWeight: 700,
          fontFamily: 'inherit',
          background: palette.primary,
          color: '#fff',
          border: 'none',
          borderRadius: 11,
          padding: '14px 24px',
          cursor: state.status === 'sending' ? 'progress' : 'pointer',
          justifySelf: 'start',
        }}
      >
        {state.status === 'sending' ? t('sending') : t('submit')}
      </button>
    </form>
  );
}

const controlStyle: React.CSSProperties = {
  width: '100%',
  fontSize: 14.5,
  fontFamily: 'inherit',
  color: palette.deep,
  padding: '11px 14px',
  borderRadius: 10,
  border: `1px solid ${palette.hairlineStrong}`,
  background: '#fff',
};

function Field({
  label,
  htmlFor,
  error,
  required,
  children,
}: {
  label: string;
  htmlFor: string;
  error?: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label
        htmlFor={htmlFor}
        style={{ display: 'block', fontSize: 13, fontWeight: 600, marginBottom: 6 }}
      >
        {label}
        {required && (
          <span aria-hidden style={{ color: palette.accent }}>
            {' '}
            *
          </span>
        )}
      </label>
      {children}
      {error && (
        <p
          role="alert"
          style={{ margin: '6px 0 0', fontSize: 12.5, color: '#B03A24' }}
          data-testid={`error-${htmlFor}`}
        >
          {error}
        </p>
      )}
    </div>
  );
}

function emptyToNull(value: FormDataEntryValue | null): string | null {
  const s = typeof value === 'string' ? value.trim() : '';
  return s === '' ? null : s;
}
