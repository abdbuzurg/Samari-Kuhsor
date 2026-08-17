'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';
import * as Icons from 'lucide-react';

import { visibleGroups, type ModuleKey } from '@/lib/modules';

/**
 * The sidebar, reproducing design/Samari-Kuhsor-Green-CRM.html.
 *
 * Layout invariants that must not regress (CLAUDE.md §5, HANDOFF-CRM-CONTEXT.md:163):
 *   - width exactly 252px
 *   - the brand block is exactly 64px, matching the top bar, so the divider is
 *     continuous across the seam
 *   - fill is --sk-chrome (#124524), the only place green is decorative
 *
 * Icons come from the lucide-react package, not the unpkg CDN the prototype
 * used: a CRM on a single Dushanbe box must not depend on unpkg being reachable
 * from Khorog, and the CDN script was recorded as stalling first paint
 * (HANDOFF-CRM-CONTEXT.md:135).
 */

/** Maps the prototype's lucide names to the React components. */
function ModuleIcon({ name, size = 16 }: { name: string; size?: number }) {
  const pascal = name
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('');
  const Icon = (Icons as unknown as Record<string, React.ComponentType<Icons.LucideProps>>)[pascal]
    ?? Icons.Package;
  return <Icon size={size} strokeWidth={2} aria-hidden />;
}

export interface SidebarProps {
  /** The resolved permission list from /auth/me. */
  permissions: readonly string[];
  /** Pending-work counts per module, from the alerts service. */
  counts?: Partial<Record<ModuleKey, number>>;
}

export function Sidebar({ permissions, counts = {} }: SidebarProps) {
  const t = useTranslations();
  const pathname = usePathname();
  const groups = visibleGroups(permissions);

  return (
    <aside
      className="w-[252px] shrink-0 flex flex-col text-white"
      style={{ background: 'var(--sk-chrome)' }}
      data-testid="sidebar"
    >
      {/* Exactly 64px: the top bar is the same height so the divider is
          continuous across the seam. */}
      <div className="h-16 shrink-0 flex items-center gap-3 px-4 border-b border-white/10">
        <div
          className="w-10 h-10 shrink-0 grid place-items-center rounded-sm text-[15px]"
          style={{
            background: 'var(--color-accent)',
            fontFamily: 'var(--font-heading)',
            fontWeight: 'var(--font-heading-weight)',
          }}
        >
          СК
        </div>
        <div className="min-w-0">
          <div
            className="text-[15px] leading-tight truncate"
            style={{ fontFamily: 'var(--font-heading)', fontWeight: 'var(--font-heading-weight)' }}
          >
            Самари Кӯҳсор
          </div>
          <div className="text-[11px] text-white/60 truncate">{t('brandSub')}</div>
        </div>
      </div>

      <nav className="flex-1 overflow-y-auto py-3" aria-label={t('mod.dashboard')}>
        {groups.map((group) => (
          <div key={group.key} className="mb-1">
            <div className="px-4 pt-3 pb-1.5 text-[10px] uppercase tracking-[0.14em] text-white/45">
              {t(`group.${group.key}`)}
            </div>
            {group.modules.map((mod) => {
              const active = pathname === mod.href || pathname.startsWith(`${mod.href}/`);
              const count = counts[mod.key];
              return (
                <Link
                  key={mod.key}
                  href={mod.href}
                  aria-current={active ? 'page' : undefined}
                  className={`relative flex items-center gap-2.5 px-4 py-2 text-[13px] transition-colors ${
                    active ? 'bg-white/10 text-white' : 'text-white/75 hover:bg-white/5 hover:text-white'
                  }`}
                >
                  {active && (
                    <span
                      className="absolute left-0 top-0 bottom-0 w-[3px]"
                      style={{ background: 'var(--sk-chrome-mark)' }}
                      aria-hidden
                    />
                  )}
                  <ModuleIcon name={mod.icon} />
                  <span className="flex-1 truncate">{t(`mod.${mod.key}`)}</span>
                  {count !== undefined && count > 0 && (
                    <span
                      className="shrink-0 min-w-5 px-1.5 h-5 grid place-items-center rounded-full text-[11px] tabular-nums"
                      style={
                        active
                          ? { background: 'var(--sk-chrome-mark)', color: 'var(--sk-chrome)' }
                          : { background: 'rgba(255,255,255,.14)', color: '#fff' }
                      }
                    >
                      {count}
                    </span>
                  )}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="shrink-0 px-4 py-3 border-t border-white/10 flex items-center gap-2 text-[11px] text-white/55">
        <span
          className="w-1.5 h-1.5 rounded-full"
          style={{ background: 'var(--sk-chrome-mark)' }}
          aria-hidden
        />
        {t('online')}
      </div>
    </aside>
  );
}
