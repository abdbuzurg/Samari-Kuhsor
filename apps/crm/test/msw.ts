import { setupServer } from 'msw/node';
import { http, HttpResponse, delay } from 'msw';
import type { User } from '@samari/types';

export const server = setupServer();

/** A user with the seed "Склад" role's permissions (docs/04-RBAC.md §4). */
export const warehouseUser: User = {
  id: '01a00f63-0000-7000-8000-000000000001',
  email: 'warehouse@samari-kuhsor.tj',
  full_name: 'А. Раҳимов',
  roles: [{ key: 'warehouse', name_ru: 'Склад', name_tg: 'Анбор', name_en: 'Warehouse' }],
  permissions: [
    'dashboard:read',
    'items:read',
    'inventory:manage',
    'procurement:manage',
    'production:read',
    'quality:read',
    'logistics:read',
    'equipment:read',
    'documents:read',
  ],
};

export const adminUser: User = {
  ...warehouseUser,
  email: 'admin@samari-kuhsor.tj',
  roles: [{ key: 'admin', name_ru: 'Администратор', name_tg: 'Маъмур', name_en: 'Administrator' }],
  permissions: [
    'dashboard:manage', 'crm:manage', 'inquiries:manage', 'items:manage',
    'inventory:manage', 'procurement:manage', 'procurement:approve',
    'production:manage', 'quality:manage', 'quality:approve',
    'logistics:manage', 'hr:manage', 'equipment:manage',
    'documents:manage', 'documents:approve', 'cms:manage', 'cms:approve',
    'admin:manage', 'audit:read',
  ],
};

/**
 * A QC technician who may record tests but not release stock.
 *
 * The distinction is the whole point of the Качество module: recording a result
 * is data entry, releasing a batch is an act of authority (docs/04-RBAC.md §3).
 * Two users are needed to prove the screen respects it.
 */
export const qcTechnician: User = {
  ...warehouseUser,
  email: 'qc@samari-kuhsor.tj',
  full_name: 'М. Назарова',
  roles: [{ key: 'quality', name_ru: 'Качество', name_tg: 'Сифат', name_en: 'Quality' }],
  permissions: ['dashboard:read', 'items:read', 'quality:manage', 'inventory:read'],
};

/** The same technician, plus the authority to release and to recall. */
export const qcLead: User = {
  ...qcTechnician,
  email: 'qclead@samari-kuhsor.tj',
  full_name: 'С. Одинаев',
  permissions: [...qcTechnician.permissions, 'quality:approve', 'audit:read'],
};

/** A user an administrator created but forgot to assign a role. */
export const noRoleUser: User = { ...warehouseUser, roles: [], permissions: [] };

export const session = {
  loaded: (user: User = warehouseUser) =>
    http.get('/api/auth/me', () => HttpResponse.json({ data: user })),

  loading: () =>
    http.get('/api/auth/me', async () => {
      await delay('infinite');
      return HttpResponse.json({ data: warehouseUser });
    }),

  unauthenticated: () =>
    http.get('/api/auth/me', () =>
      HttpResponse.json(
        { error: { code: 'unauthenticated', message: 'Требуется вход в систему' } },
        { status: 401 },
      ),
    ),

  serverError: () =>
    http.get('/api/auth/me', () =>
      HttpResponse.json(
        { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
        { status: 500 },
      ),
    ),
};

export { http, HttpResponse };
