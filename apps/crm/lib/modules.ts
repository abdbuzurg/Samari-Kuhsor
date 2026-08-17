/**
 * The navigation model, transcribed from the approved prototype
 * (design/Samari-Kuhsor-Green-CRM.html:549 GROUPS / :555 MODICON) and
 * docs/05-MODULES.md §1.
 *
 * Order and grouping are part of the visual contract — reproduce, do not improve
 * (CLAUDE.md §5).
 */

/** Resource keys, matching docs/04-RBAC.md §2 exactly. */
export type ModuleKey =
  | 'dashboard'
  | 'crm'
  | 'inquiries'
  | 'items'
  | 'inventory'
  | 'procurement'
  | 'production'
  | 'quality'
  | 'logistics'
  | 'finance'
  | 'hr'
  | 'equipment'
  | 'documents';

export type GroupKey = 'overview' | 'sales' | 'ops' | 'admin';

export interface NavModule {
  key: ModuleKey;
  href: string;
  /** Lucide icon name, from the prototype's MODICON map. */
  icon: string;
  /**
   * Hidden regardless of permission. Финансы и бюджет is deferred until the
   * client answers register question Q2, so it must not appear in the nav even
   * for an administrator (D2, docs/05-MODULES.md:22).
   */
  hiddenAtLaunch?: boolean;
}

export interface NavGroup {
  key: GroupKey;
  modules: NavModule[];
}

export const NAV_GROUPS: NavGroup[] = [
  {
    key: 'overview',
    modules: [{ key: 'dashboard', href: '/', icon: 'gauge' }],
  },
  {
    key: 'sales',
    modules: [
      { key: 'crm', href: '/crm', icon: 'users' },
      { key: 'inquiries', href: '/inquiries', icon: 'globe' },
      { key: 'items', href: '/items', icon: 'package' },
    ],
  },
  {
    key: 'ops',
    modules: [
      { key: 'inventory', href: '/inventory', icon: 'warehouse' },
      { key: 'procurement', href: '/procurement', icon: 'shopping-cart' },
      { key: 'production', href: '/production', icon: 'factory' },
      { key: 'quality', href: '/quality', icon: 'shield' },
      { key: 'logistics', href: '/logistics', icon: 'truck' },
    ],
  },
  {
    key: 'admin',
    modules: [
      { key: 'finance', href: '/finance', icon: 'wallet', hiddenAtLaunch: true },
      { key: 'hr', href: '/hr', icon: 'contact' },
      { key: 'equipment', href: '/equipment', icon: 'wrench' },
      { key: 'documents', href: '/documents', icon: 'file-text' },
    ],
  },
];

/**
 * Filters the nav to what a user may read.
 *
 * `manage` implies `read`, which is why this checks the resolved permission list
 * for either. A user with no permission on a module never sees it
 * (docs/05-MODULES.md:25) — but hiding is cosmetic, and the server still refuses
 * the request (docs/04-RBAC.md:120).
 */
export function visibleGroups(permissions: readonly string[]): NavGroup[] {
  const granted = new Set(permissions);
  const canRead = (key: ModuleKey) =>
    granted.has(`${key}:read`) || granted.has(`${key}:manage`);

  return NAV_GROUPS.map((group) => ({
    ...group,
    modules: group.modules.filter((m) => !m.hiddenAtLaunch && canRead(m.key)),
  })).filter((group) => group.modules.length > 0);
}
