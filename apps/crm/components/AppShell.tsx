'use client';

import { useRouter } from 'next/navigation';
import { useLocale } from 'next-intl';
import { useTransition, type ReactNode } from 'react';

import { Sidebar } from '@/components/Sidebar';
import { TopBar } from '@/components/TopBar';
import { useSession, useLogout, can } from '@/lib/session';
import { setLocale } from '@/app/actions';
import type { Locale } from '@/i18n/config';
import type { Role } from '@samari/types';
import type { ModuleKey } from '@/lib/modules';

/**
 * The application shell: sidebar + top bar + content, per docs/05-MODULES.md §1.
 *
 * The shell owns the four data states CLAUDE.md §7 requires, because the session
 * query is what every screen depends on:
 *
 *   loading  — the session is resolving
 *   error    — unauthenticated, so redirect to login
 *   empty    — authenticated but with no permissions at all
 *   loaded   — the normal case
 *
 * "Empty" is a real state here, not a theoretical one: an administrator can
 * create a user and forget to assign a role, and that user must get an
 * explanation rather than an empty sidebar and a blank page.
 */
/**
 * Role names are content, not UI chrome: administrators create roles and name
 * them (CLAUDE.md §6), so the name comes from the record rather than from the
 * message dictionary. Falls back to Russian, matching the rule for every other
 * translatable record (docs/02-SCHEMA.md:53).
 */
function roleName(role: Role | undefined, locale: Locale): string {
  if (!role) return '';
  const byLocale = { ru: role.name_ru, tg: role.name_tg, en: role.name_en };
  return byLocale[locale] || role.name_ru || role.key;
}

export function AppShell({
  children,
  counts,
}: {
  children: ReactNode;
  counts?: Partial<Record<ModuleKey, number>>;
}) {
  const router = useRouter();
  const locale = useLocale() as Locale;
  const [, startTransition] = useTransition();

  const session = useSession();
  const logout = useLogout();

  if (session.isLoading) {
    return (
      <div
        className="min-h-screen grid place-items-center text-sm"
        style={{ color: 'color-mix(in srgb, var(--color-text) 55%, transparent)' }}
        role="status"
        aria-live="polite"
      >
        Загрузка…
      </div>
    );
  }

  if (session.isError || !session.data) {
    // Not an error screen: an unresolvable session means "not signed in", and the
    // only useful thing to do is offer the login page.
    return (
      <div className="min-h-screen grid place-items-center">
        <button
          type="button"
          className="btn"
          style={{ background: 'var(--color-accent)', color: 'var(--color-bg)' }}
          onClick={() => router.replace('/login')}
        >
          Войти в систему
        </button>
      </div>
    );
  }

  const user = session.data;

  const handleLocaleChange = (next: Locale) => {
    startTransition(async () => {
      await setLocale(next);
      router.refresh();
    });
  };

  const handleLogout = () => {
    logout.mutate(undefined, { onSettled: () => router.replace('/login') });
  };

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar permissions={user.permissions} counts={counts} />
      <div className="flex-1 flex flex-col min-w-0">
        <TopBar
          userName={user.full_name}
          roleName={roleName(user.roles[0], locale)}
          locale={locale}
          canAdminister={can(user.permissions, 'admin', 'manage')}
          canAudit={can(user.permissions, 'audit', 'read')}
          canEditContent={can(user.permissions, 'cms', 'read')}
          onLocaleChange={handleLocaleChange}
          onLogout={handleLogout}
        />
        <main className="flex-1 overflow-y-auto p-6" id="sk-content">
          {user.permissions.length === 0 ? (
            <div className="card p-6 max-w-lg">
              <h1 className="text-lg mb-2" style={{ fontFamily: 'var(--font-heading)' }}>
                Доступ не настроен
              </h1>
              <p className="muted text-[13px]">
                Вашей учётной записи не назначена ни одна роль. Обратитесь к администратору
                системы.
              </p>
            </div>
          ) : (
            children
          )}
        </main>
      </div>
    </div>
  );
}
