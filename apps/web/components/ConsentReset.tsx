'use client';

import { useState } from 'react';
import { useTranslations } from 'next-intl';

import { resetConsent } from '@/lib/consent';
import { palette } from '@/lib/palette';

/**
 * A way back.
 *
 * Until D12 there was none: decline once and the banner never returned, so a
 * visitor could not change their mind and there was nothing for a regulator to
 * look at. Clearing the stored answer puts the banner back on the next render,
 * which is the same mechanism as never having been asked.
 */
export function ConsentReset() {
  const t = useTranslations('consent');
  const [done, setDone] = useState(false);

  return (
    <p>
      <button
        type="button"
        data-testid="consent-reset"
        onClick={() => {
          resetConsent();
          setDone(true);
        }}
        style={{
          font: 'inherit',
          fontWeight: 700,
          color: palette.primaryHover,
          background: 'none',
          border: 'none',
          borderBottom: `2px solid ${palette.accent}`,
          padding: '0 0 2px',
          cursor: 'pointer',
        }}
      >
        {t('reset')}
      </button>
      {done && (
        <span style={{ marginLeft: 10, color: palette.muted }} role="status">
          {t('resetDone')}
        </span>
      )}
    </p>
  );
}
