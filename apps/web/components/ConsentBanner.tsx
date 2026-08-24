'use client';

import { useTranslations } from 'next-intl';

import { useConsent, writeConsent } from '@/lib/consent';
import { palette } from '@/lib/palette';

/**
 * The analytics consent banner.
 *
 * Shown once, on the `unknown` state only. Both buttons are real choices with
 * equal weight — no pre-ticked box, no "accept" styled as the only way forward,
 * and no dismissal that silently counts as consent.
 *
 * Nothing is tracked before a choice is made: <Analytics> mounts no script at
 * all until consent is 'granted', so declining is the same as never asking.
 */
export function ConsentBanner() {
  const t = useTranslations('consent');
  const consent = useConsent();

  if (consent !== 'unknown') return null;

  return (
    <div
      role="dialog"
      aria-live="polite"
      aria-label={t('title')}
      data-testid="consent-banner"
      className="skc-fixed-inset"
      style={{
        position: 'fixed',
        left: 16,
        right: 16,
        bottom: 16,
        zIndex: 80,
        maxWidth: 620,
        margin: '0 auto',
        background: '#fff',
        border: `1px solid ${palette.hairlineStrong}`,
        borderRadius: 14,
        padding: '18px 20px',
        boxShadow: '0 12px 34px rgba(20,28,34,.16)',
        display: 'flex',
        flexWrap: 'wrap',
        gap: 14,
        alignItems: 'center',
      }}
    >
      <p style={{ margin: 0, fontSize: 13.5, lineHeight: 1.55, flex: '1 1 260px' }}>
        {t('body')}
      </p>
      <div style={{ display: 'flex', gap: 8, marginLeft: 'auto' }}>
        <button
          type="button"
          onClick={() => writeConsent('denied')}
          style={{
            fontSize: 13.5,
            fontWeight: 700,
            padding: '12px 18px',
            minHeight: 44,
            borderRadius: 10,
            border: `1.5px solid ${palette.hairlineStrong}`,
            background: 'transparent',
            color: palette.deep,
            cursor: 'pointer',
            fontFamily: 'inherit',
          }}
        >
          {t('decline')}
        </button>
        <button
          type="button"
          onClick={() => writeConsent('granted')}
          style={{
            fontSize: 13.5,
            fontWeight: 700,
            padding: '12px 18px',
            minHeight: 44,
            borderRadius: 10,
            border: 'none',
            background: palette.primary,
            color: '#fff',
            cursor: 'pointer',
            fontFamily: 'inherit',
          }}
        >
          {t('accept')}
        </button>
      </div>
    </div>
  );
}
