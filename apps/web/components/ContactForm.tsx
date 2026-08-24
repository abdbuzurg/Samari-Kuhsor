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

/**
 * The six reasons the design offers, mapped to the five types the API accepts.
 *
 * `supplier` and `media` have no backend type of their own — the enquiry table's
 * CHECK constraint knows five. Rather than invent two, both submit as `contact`
 * with the reason named in the message, which is where a salesperson reads it
 * anyway. Adding enum values to a live constraint for a distinction nobody
 * filters on would be the more expensive answer.
 */
const REASONS = [
  { key: 'wholesale', type: 'wholesale', label: 'typeWholesale' },
  { key: 'distributor', type: 'distributor', label: 'typeDistributor' },
  { key: 'contact', type: 'contact', label: 'typeContact' },
  { key: 'supplier', type: 'contact', label: 'typeSupplier' },
  { key: 'job', type: 'job', label: 'typeJobShort' },
  { key: 'media', type: 'contact', label: 'typeMedia' },
] as const;

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
        body: JSON.stringify(payloadFrom(form)),
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

      <Field label={t('type')} htmlFor="reason" error={fieldErrors.type}>
        <select id="reason" name="reason" defaultValue="wholesale" style={controlStyle}>
          {REASONS.map((r) => (
            <option key={r.key} value={r.key}>
              {t(r.label)}
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

      {/* Email and phone are separate fields in the design, and they submit as
          one `contact` string. The backend stores a single contact because that
          is what a salesperson calls back on — but asking for "телефон или
          e-mail" in one box gets you half of each. */}
      <Field label={t('email')} htmlFor="email" error={fieldErrors.contact} required>
        <input
          id="email"
          name="email"
          type="email"
          required
          style={controlStyle}
          autoComplete="email"
        />
      </Field>

      <Field label={t('phone')} htmlFor="phone" error={fieldErrors.phone} required>
        <input id="phone" name="phone" required style={controlStyle} autoComplete="tel" />
      </Field>

      <Field label={t('country')} htmlFor="country" error={fieldErrors.country} required>
        <input
          id="country"
          name="country"
          required
          style={controlStyle}
          autoComplete="country-name"
        />
      </Field>

      <Field label={t('instagram')} htmlFor="instagram" error={fieldErrors.instagram}>
        <input
          id="instagram"
          name="instagram"
          type="url"
          placeholder="https://instagram.com/…"
          style={controlStyle}
        />
      </Field>

      <Field label={t('facebook')} htmlFor="facebook" error={fieldErrors.facebook}>
        <input
          id="facebook"
          name="facebook"
          type="url"
          placeholder="https://facebook.com/…"
          style={controlStyle}
        />
      </Field>

      <Field label={t('message')} htmlFor="message" error={fieldErrors.message} required>
        <textarea id="message" name="message" rows={4} required style={controlStyle} />
      </Field>

      {/* Required, and unticked by default. A pre-ticked consent box is not
          consent, and this is the one field on the page that has to survive a
          regulator reading it. */}
      <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start', fontSize: 13 }}>
        <input
          type="checkbox"
          name="consent"
          required
          className="skc-tap-check"
          style={{ marginTop: 3 }}
        />
        <span>
          {t('consent')} <span aria-hidden style={{ color: palette.accent }}>*</span>
        </span>
      </label>

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

/**
 * Builds the API payload from the form.
 *
 * Everything the design collects but the API has no column for — country, the
 * two social URLs, and the reason when it is one of the two that map onto
 * `contact` — is appended to the message rather than dropped. A field the
 * visitor filled in and nobody ever reads is worse than not asking.
 */
function payloadFrom(form: FormData): Record<string, unknown> {
  const reasonKey = String(form.get('reason') ?? 'contact');
  const reason = REASONS.find((r) => r.key === reasonKey) ?? REASONS[2];

  const email = str(form.get('email'));
  const phone = str(form.get('phone'));

  const extras: string[] = [];
  const country = str(form.get('country'));
  const instagram = str(form.get('instagram'));
  const facebook = str(form.get('facebook'));
  if (country) extras.push(`Страна: ${country}`);
  if (instagram) extras.push(`Instagram: ${instagram}`);
  if (facebook) extras.push(`Facebook: ${facebook}`);
  // Named explicitly when the reason does not survive the type mapping, so the
  // recipient still knows a supplier or a journalist wrote in.
  if (reason.type !== reason.key) extras.push(`Тип обращения: ${reasonKey}`);

  const body = [str(form.get('message')), ...extras].filter(Boolean).join('\n\n');

  return {
    type: reason.type,
    name: str(form.get('name')),
    company: emptyToNull(form.get('company')),
    // Both, so nobody has to guess which one the visitor meant.
    contact: [email, phone].filter(Boolean).join(' · '),
    message: body || null,
  };
}

function str(value: FormDataEntryValue | null): string {
  return typeof value === 'string' ? value.trim() : '';
}

function emptyToNull(value: FormDataEntryValue | null): string | null {
  const s = typeof value === 'string' ? value.trim() : '';
  return s === '' ? null : s;
}
