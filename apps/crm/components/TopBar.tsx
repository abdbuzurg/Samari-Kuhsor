'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import Link from 'next/link';
import {
  Search,
  Bell,
  Settings,
  ChevronDown,
  User,
  LogOut,
  ScrollText,
  FileText,
  Menu,
} from 'lucide-react';

import { locales, localeLabels, type Locale } from '@/i18n/config';

/**
 * The top bar, reproducing design/Samari-Kuhsor-Green-CRM.html:700.
 *
 * Exactly 64px tall, matching the sidebar's brand block so the divider is
 * continuous across the seam (CLAUDE.md §5).
 *
 * Popovers explicitly reset text colour: everything inside .sk-top inherits
 * white, so a light surface placed there needs its own colour or it renders
 * white-on-white — a real defect found in prototype review
 * (HANDOFF-CRM-CONTEXT.md:385).
 */

export interface TopBarProps {
  userName: string;
  roleName: string;
  locale: Locale;
  unreadCount?: number;
  /** Shows the Администрирование gear. Cosmetic — Go refuses regardless. */
  canAdminister?: boolean;
  /** Shows Журнал действий in the user menu. Guarded on audit:read, which is a
   *  different authority from admin:manage: reading the trail and changing who
   *  may do what are not the same power. */
  canAudit?: boolean;
  /** Shows Контент сайта. The CMS is not one of the prototype's thirteen
   *  sidebar modules, so like admin it lives in the top bar. */
  canEditContent?: boolean;
  onLocaleChange: (locale: Locale) => void;
  onLogout: () => void;
  onSearch?: (query: string) => void;
}

export function TopBar({
  userName,
  roleName,
  locale,
  unreadCount = 0,
  canAdminister = false,
  canAudit = false,
  canEditContent = false,
  onLocaleChange,
  onLogout,
  onSearch,
  onOpenNav,
}: TopBarProps & { onOpenNav?: () => void }) {
  const t = useTranslations();
  const [menuOpen, setMenuOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [, startTransition] = useTransition();

  const initials = userName
    .split(' ')
    .map((part) => part.charAt(0))
    .slice(0, 2)
    .join('');

  return (
    <header
      className="h-16 shrink-0 flex items-center gap-4 px-5 text-white"
      style={{ background: 'var(--sk-chrome)' }}
      data-testid="topbar"
    >
      {/* Below lg the sidebar is a drawer, so it needs a way in. Hidden at
          desktop, where the nav is always visible. */}
      {onOpenNav && (
        <button
          type="button"
          className="lg:hidden shrink-0 p-1.5 -ml-1 rounded-sm hover:bg-white/10"
          aria-label="Открыть меню"
          data-testid="open-nav"
          onClick={onOpenNav}
        >
          <Menu size={20} aria-hidden />
        </button>
      )}

      <div className="relative flex-1 max-w-[420px]">
        {/* 16x16 at left:12px with the input padded to 38px, per the prototype's
            search-icon invariant (HANDOFF-CRM-CONTEXT.md:166). */}
        <Search
          size={16}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-white/50 pointer-events-none"
          aria-hidden
        />
        <input
          type="search"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            startTransition(() => onSearch?.(e.target.value));
          }}
          placeholder={t('search')}
          aria-label={t('search')}
          className="w-full h-9 pl-[38px] pr-3 text-[13px] rounded-sm bg-white/10 text-white placeholder:text-white/45 border border-white/15 focus:outline-none focus:border-white/35"
        />
      </div>

      <div className="flex-1" />

      {/* ТҶ / РУ / EN. The Tajik code is `tg`; only the label is ТҶ (C2). */}
      <div
        className="flex items-center rounded-sm border border-white/15 overflow-hidden"
        role="group"
        aria-label="Язык интерфейса"
      >
        {locales.map((code) => (
          <button
            key={code}
            type="button"
            onClick={() => onLocaleChange(code)}
            aria-pressed={locale === code}
            className={`px-2.5 h-7 text-[11px] tracking-wide transition-colors ${
              locale === code ? 'bg-white/20 text-white' : 'text-white/65 hover:text-white'
            }`}
          >
            {localeLabels[code]}
          </button>
        ))}
      </div>

      <button
        type="button"
        className="relative w-9 h-9 grid place-items-center rounded-sm hover:bg-white/10"
        aria-label={t('notifs')}
      >
        <Bell size={17} aria-hidden />
        {unreadCount > 0 && (
          <span
            className="absolute top-1 right-1 min-w-4 h-4 px-1 grid place-items-center rounded-full text-[10px] tabular-nums"
            style={{ background: 'var(--sk-danger)', color: '#fff' }}
          >
            {unreadCount}
          </span>
        )}
      </button>

      {/* Администрирование lives here rather than in the sidebar. The
          prototype's nav is exactly thirteen modules and admin is not one of
          them (design/Samari-Kuhsor-Green-CRM.html:549); adding a fourteenth
          would be a change the client did not ask for. The gear is where the
          prototype already put settings, and it is hidden entirely from a user
          who cannot administer — hiding is cosmetic, but a control that always
          fails is its own defect. */}
      {canAdminister && (
        <Link
          href="/admin"
          className="w-9 h-9 grid place-items-center rounded-sm hover:bg-white/10"
          aria-label={t('settings')}
        >
          <Settings size={17} aria-hidden />
        </Link>
      )}

      <div className="relative">
        <button
          type="button"
          onClick={() => setMenuOpen((open) => !open)}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          className="flex items-center gap-2.5 pl-1 pr-2 h-10 rounded-sm hover:bg-white/10"
        >
          <span
            className="w-[30px] h-[30px] grid place-items-center rounded-full text-[12px]"
            style={{
              background: 'var(--color-accent)',
              fontFamily: 'var(--font-heading)',
              fontWeight: 'var(--font-heading-weight)',
            }}
          >
            {initials}
          </span>
          <span className="text-left leading-tight hidden sm:block">
            <span className="block text-[13px]">{userName}</span>
            <span className="block text-[11px] text-white/60">{roleName}</span>
          </span>
          <ChevronDown size={14} aria-hidden />
        </button>

        {menuOpen && (
          // Explicit colour reset: this light surface sits inside the chrome,
          // which sets white text on everything it contains.
          <div
            role="menu"
            className="absolute right-0 top-full mt-1 w-52 rounded-sm shadow-lg z-50 py-1"
            style={{ background: 'var(--color-bg)', color: 'var(--color-text)' }}
          >
            <button
              type="button"
              role="menuitem"
              className="w-full flex items-center gap-2 px-3 py-2 text-[13px] hover:bg-black/5"
            >
              <User size={15} aria-hidden />
              {t('profile')}
            </button>
            {canEditContent && (
              <Link
                href="/cms"
                role="menuitem"
                className="w-full flex items-center gap-2 px-3 py-2 text-[13px] hover:bg-black/5"
              >
                <FileText size={15} aria-hidden />
                Контент сайта
              </Link>
            )}
            {canAudit && (
              <Link
                href="/audit"
                role="menuitem"
                className="w-full flex items-center gap-2 px-3 py-2 text-[13px] hover:bg-black/5"
              >
                <ScrollText size={15} aria-hidden />
                Журнал действий
              </Link>
            )}
            <button
              type="button"
              role="menuitem"
              onClick={onLogout}
              className="w-full flex items-center gap-2 px-3 py-2 text-[13px] hover:bg-black/5"
            >
              <LogOut size={15} aria-hidden />
              {t('logout')}
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
