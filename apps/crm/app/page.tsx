'use client';

import { useTranslations } from 'next-intl';

import { AppShell } from '@/components/AppShell';

/**
 * Панель управления — built LAST (docs/05-MODULES.md:60), because it aggregates
 * every module beneath it. This is the shell landing page until T33.
 *
 * Note what is deliberately absent: the prototype's sample figures. 05-MODULES.md:70
 * is explicit — "Do not fabricate figures, and do not carry the prototype's sample
 * numbers into production." On opening day these panels are genuinely empty, and
 * showing 2 480 000 c. of revenue for a factory that has produced nothing would
 * be a lie the client would reasonably act on.
 */
export default function DashboardPage() {
  const t = useTranslations();

  return (
    <AppShell>
      <div className="mb-5">
        <div className="text-[11px] uppercase tracking-[0.18em] muted">{t('group.overview')}</div>
        <h1 className="text-[27px] leading-tight mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
          {t('mod.dashboard')}
        </h1>
      </div>

      <div className="card p-6">
        <p className="text-[13px] muted">
          Панель управления собирает данные из всех модулей и будет заполнена по мере их
          подключения. Показатели появятся, когда появятся первые операции.
        </p>
      </div>
    </AppShell>
  );
}
