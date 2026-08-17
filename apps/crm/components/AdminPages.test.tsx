import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import AdminPage from '@/app/admin/page';
import AuditPage from '@/app/audit/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { AdminUserRow, AuditEntry, PermissionCatalogue, RoleDetail } from '@samari/types';

/**
 * Администрирование and Журнал действий.
 *
 * What is worth testing here is not the table layout. It is that the screen
 * tells the truth about two things a user cannot otherwise see: that the last
 * administrator is protected, and that the audit trail has no edit path.
 */

let client: QueryClient;

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
});

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </NextIntlClientProvider>,
  );
}

const ADMIN_ROLE: RoleDetail = {
  id: '018f3c9e-0000-7000-8000-000000000001',
  key: 'admin',
  name: 'Администратор',
  permissions: ['admin:manage', 'items:manage'],
  user_count: 1,
  version: 1,
};

const WAREHOUSE_ROLE: RoleDetail = {
  id: '018f3c9e-0000-7000-8000-000000000002',
  key: 'warehouse',
  name: 'Склад',
  permissions: ['inventory:manage'],
  user_count: 2,
  version: 1,
};

const ADMIN: AdminUserRow = {
  id: '018f3c9e-0000-7000-8000-0000000000a1',
  email: 'admin@samari-kuhsor.tj',
  full_name: 'Ф. Давлатова',
  is_active: true,
  status: { key: 'active', label: 'Активен', level: 'ok' },
  roles: ['admin'],
  version: 1,
};

const OPERATOR: AdminUserRow = {
  id: '018f3c9e-0000-7000-8000-0000000000a2',
  email: 'warehouse@samari-kuhsor.tj',
  full_name: 'С. Назаров',
  is_active: true,
  status: { key: 'active', label: 'Активен', level: 'ok' },
  roles: ['warehouse'],
  version: 1,
};

const CATALOGUE: PermissionCatalogue = {
  resources: [
    { key: 'items', actions: ['read', 'manage', 'approve'] },
    { key: 'inventory', actions: ['read', 'manage', 'approve'] },
    { key: 'admin', actions: ['read', 'manage', 'approve'] },
  ],
};

function adminHandlers(opts: { roles?: RoleDetail[]; users?: AdminUserRow[] } = {}) {
  return [
    http.get('/api/admin/roles', () =>
      HttpResponse.json({ data: opts.roles ?? [ADMIN_ROLE, WAREHOUSE_ROLE] }),
    ),
    http.get('/api/admin/users', () =>
      HttpResponse.json({
        data: opts.users ?? [ADMIN, OPERATOR],
        meta: { page: 1, per_page: 25, total: 2, total_pages: 1 },
      }),
    ),
    http.get('/api/admin/permissions', () => HttpResponse.json({ data: CATALOGUE })),
  ];
}

// ---------------------------------------------------------------------------
// Администрирование
// ---------------------------------------------------------------------------

describe('Администрирование', () => {
  it('lists roles and users once loaded', async () => {
    server.use(session.loaded(adminUser), ...adminHandlers());
    wrap(<AdminPage />);
    // By role, because the top bar also renders the signed-in user's role name.
    expect(await screen.findByRole('button', { name: 'Администратор' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Склад' })).toBeInTheDocument();
    expect(screen.getByText('С. Назаров')).toBeInTheDocument();
  });

  it('shows loading states for both sections', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/admin/roles', async () => {
        await delay('infinite');
        return HttpResponse.json({ data: [] });
      }),
      http.get('/api/admin/users', async () => {
        await delay('infinite');
        return HttpResponse.json({ data: [] });
      }),
      http.get('/api/admin/permissions', () => HttpResponse.json({ data: CATALOGUE })),
    );
    wrap(<AdminPage />);
    await waitFor(() => {
      expect(screen.getByTestId('roles-loading')).toBeInTheDocument();
    });
    expect(screen.getByTestId('users-loading')).toBeInTheDocument();
  });

  it('shows empty states rather than an empty table', async () => {
    server.use(session.loaded(adminUser), ...adminHandlers({ roles: [], users: [] }));
    wrap(<AdminPage />);
    expect(await screen.findByTestId('roles-empty')).toBeInTheDocument();
    expect(screen.getByTestId('users-empty')).toBeInTheDocument();
  });

  it('shows error states when the requests fail', async () => {
    const fail = () =>
      HttpResponse.json(
        { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
        { status: 500 },
      );
    server.use(
      session.loaded(adminUser),
      http.get('/api/admin/roles', fail),
      http.get('/api/admin/users', fail),
      http.get('/api/admin/permissions', fail),
    );
    wrap(<AdminPage />);
    expect(await screen.findByTestId('roles-error')).toBeInTheDocument();
    expect(screen.getByTestId('users-error')).toBeInTheDocument();
  });

  // The last-admin guard is invisible until it fires, and a manager who cannot
  // see it reads the server's refusal as a bug.
  it('warns while only one administrator remains', async () => {
    server.use(session.loaded(adminUser), ...adminHandlers());
    wrap(<AdminPage />);
    expect(await screen.findByTestId('last-admin-notice')).toBeInTheDocument();
  });

  it('drops the warning once a second administrator exists', async () => {
    const second: AdminUserRow = {
      ...OPERATOR,
      id: '018f3c9e-0000-7000-8000-0000000000a3',
      email: 'second@samari-kuhsor.tj',
      roles: ['admin'],
    };
    server.use(session.loaded(adminUser), ...adminHandlers({ users: [ADMIN, second] }));
    wrap(<AdminPage />);
    await screen.findByText('Ф. Давлатова');
    expect(screen.queryByTestId('last-admin-notice')).not.toBeInTheDocument();
  });

  it('surfaces the server refusal rather than failing silently', async () => {
    server.use(
      session.loaded(adminUser),
      ...adminHandlers(),
      http.put('/api/admin/users/:id/active', () =>
        HttpResponse.json(
          {
            error: {
              code: 'business_rule',
              message: 'Нельзя отключить последнего администратора системы.',
            },
          },
          { status: 409 },
        ),
      ),
    );
    wrap(<AdminPage />);
    await screen.findByText('Ф. Давлатова');

    const row = screen.getAllByTestId('user-row')[0];
    await userEvent.click(within(row).getByRole('button', { name: 'Отключить' }));

    const error = await screen.findByTestId('admin-error');
    // The server's own message, not a generic one: it names the actual rule.
    expect(error).toHaveTextContent('последнего администратора');
  });

  it('renders the permission grid from the catalogue the server sent', async () => {
    server.use(session.loaded(adminUser), ...adminHandlers());
    wrap(<AdminPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Администратор' }));

    const grid = await screen.findByTestId('permission-grid');
    // Generated from rbac's own tables, so the editor cannot offer a permission
    // the middleware does not recognise.
    expect(within(grid).getByLabelText('items:manage')).toBeChecked();
    expect(within(grid).getByLabelText('inventory:manage')).not.toBeChecked();
    expect(within(grid).getByLabelText('admin:manage')).toBeChecked();
  });

  it('does not tick `read` when `manage` is held', async () => {
    server.use(session.loaded(adminUser), ...adminHandlers());
    wrap(<AdminPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Администратор' }));

    const grid = await screen.findByTestId('permission-grid');
    // manage IMPLIES read (docs/04-RBAC.md §3), but ticking read as well would
    // teach that it must be granted separately — and unticking it would then
    // appear to remove access it does not control.
    expect(within(grid).getByLabelText('items:read')).not.toBeChecked();
    expect(within(grid).getByLabelText('items:manage')).toBeChecked();
  });

  it('sends the whole permission set on change, so a revoke is expressible', async () => {
    let sent: string[] | null = null;
    server.use(
      session.loaded(adminUser),
      ...adminHandlers(),
      http.put('/api/admin/roles/:id/permissions', async ({ request }) => {
        const body = (await request.json()) as { permissions: string[] };
        sent = body.permissions;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    wrap(<AdminPage />);
    await userEvent.click(await screen.findByRole('button', { name: 'Администратор' }));
    const grid = await screen.findByTestId('permission-grid');
    await userEvent.click(within(grid).getByLabelText('items:manage'));

    await waitFor(() => {
      expect(sent).not.toBeNull();
    });
    // The full set, minus the one just unticked — a partial update could not
    // express a revocation at all.
    expect(sent).toEqual(['admin:manage']);
  });

  it('hides the write controls from a user who may only read', async () => {
    server.use(session.loaded(warehouseUser), ...adminHandlers());
    wrap(<AdminPage />);
    await screen.findByText('Ф. Давлатова');

    // Hiding is cosmetic — Go refuses regardless (docs/04-RBAC.md:120) — but
    // offering a button that will always fail is its own defect.
    expect(screen.queryByRole('button', { name: 'Отключить' })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Журнал действий
// ---------------------------------------------------------------------------

const ENTRY: AuditEntry = {
  id: '018f3c9e-0000-7000-8000-0000000000b1',
  action: 'approve',
  resource: 'quality',
  resource_id: '018f3c9e-0000-7000-8000-0000000000c1',
  actor_id: ADMIN.id,
  actor_name: 'Ф. Давлатова',
  ip: '203.0.113.7',
  occurred_at: '2026-08-17T09:00:00Z',
  before: { status: 'quarantine' },
  after: { status: 'released' },
};

function auditHandler(rows: AuditEntry[]) {
  return http.get('/api/audit', () =>
    HttpResponse.json({
      data: rows,
      meta: { page: 1, per_page: 25, total: rows.length, total_pages: 1 },
    }),
  );
}

describe('Журнал действий', () => {
  it('renders entries with the action and module in Russian', async () => {
    server.use(session.loaded(adminUser), auditHandler([ENTRY]));
    wrap(<AuditPage />);
    await screen.findByText('Ф. Давлатова');
    // The server sends keys so the reader gets their own locale (docs/07 C3).
    expect(screen.getByText('Согласование')).toBeInTheDocument();
    expect(screen.getByText('Качество')).toBeInTheDocument();
    expect(screen.queryByText('approve')).not.toBeInTheDocument();
  });

  it('attributes an actor-less entry to the system rather than leaving it blank', async () => {
    const anonymous: AuditEntry = {
      ...ENTRY,
      actor_id: null,
      actor_name: null,
      action: 'create',
      resource: 'inquiries',
    };
    server.use(session.loaded(adminUser), auditHandler([anonymous]));
    wrap(<AuditPage />);
    // A public enquiry has no user behind it. A blank cell reads as missing
    // data; inventing a name would put someone against an action they did not
    // take.
    expect(await screen.findByText('Система')).toBeInTheDocument();
  });

  it('offers no way to edit or delete an entry', async () => {
    server.use(session.loaded(adminUser), auditHandler([ENTRY]));
    wrap(<AuditPage />);
    await screen.findByText('Ф. Давлатова');

    // audit_log has no UPDATE and no DELETE query anywhere in the backend, and
    // no deleted_at column. There is no control here because there is no
    // endpoint behind one.
    expect(screen.queryByRole('button', { name: /Удалить/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Изменить/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: /Действие/ })).not.toBeInTheDocument();
  });

  it('filters by module', async () => {
    const asked: (string | null)[] = [];
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', ({ request }) => {
        asked.push(new URL(request.url).searchParams.get('resource'));
        return HttpResponse.json({
          data: [ENTRY],
          meta: { page: 1, per_page: 25, total: 1, total_pages: 1 },
        });
      }),
    );
    wrap(<AuditPage />);
    await screen.findByText('Ф. Давлатова');

    await userEvent.type(screen.getByRole('searchbox', { name: /Фильтр по модулю/ }), 'quality');
    await waitFor(() => {
      expect(asked).toContain('quality');
    });
  });

  it('shows the empty and error states', async () => {
    server.use(session.loaded(adminUser), auditHandler([]));
    const { unmount } = wrap(<AuditPage />);
    expect(await screen.findByTestId('list-empty')).toBeInTheDocument();
    unmount();

    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', () =>
        HttpResponse.json(
          { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
          { status: 500 },
        ),
      ),
    );
    wrap(<AuditPage />);
    expect(await screen.findByTestId('list-error')).toBeInTheDocument();
  });
});
