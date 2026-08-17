'use client';

import { useTranslations } from 'next-intl';
import { useMemo, useState } from 'react';

import { AppShell } from '@/components/AppShell';
import { StatusTag } from '@/components/StatusTag';
import {
  useAdminUsers,
  usePermissionCatalogue,
  useRoles,
  useSetRolePermissions,
  useSetUserActive,
  useSetUserRoles,
} from '@/lib/operations';
import { useSession, can } from '@/lib/session';
import type { AdminUserRow, RoleDetail } from '@samari/types';

/**
 * Администрирование — roles, permissions and users (docs/04-RBAC.md §6).
 *
 * Two rules the UI must respect, both enforced in Go regardless:
 *
 *   1. The last account holding `admin:manage` cannot be deactivated or stripped
 *      of it. The server refuses; this page shows why rather than letting the
 *      user find out by failing.
 *   2. `manage` implies `read`, and `approve` implies nothing. The checkbox grid
 *      reflects that — ticking manage does not tick read, because read is
 *      already granted and a tick that means "redundant" teaches the wrong
 *      model.
 */
export default function AdminPage() {
  const t = useTranslations();
  const session = useSession();
  const mayManage = can(session.data?.permissions, 'admin', 'manage');

  const roles = useRoles();
  const users = useAdminUsers({});
  const catalogue = usePermissionCatalogue();

  const [openRole, setOpenRole] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const setPermissions = useSetRolePermissions();
  const setUserRoles = useSetUserRoles();
  const setActive = useSetUserActive();

  const roleList = roles.data?.data ?? [];
  const userList = users.data?.data ?? [];

  // The count of accounts that can still administer the system. Shown because
  // the last-admin guard is invisible until it fires, and a manager who cannot
  // see it reads the refusal as a bug.
  const adminCount = useMemo(
    () => userList.filter((u) => u.is_active && holdsAdmin(u, roleList)).length,
    [userList, roleList],
  );

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить изменения');
    }
  }

  return (
    <AppShell>
      <div className="mb-5">
        <div className="text-[11px] uppercase tracking-[0.18em] muted">{t('group.admin')}</div>
        <h1 className="text-[27px] leading-tight mt-1" style={{ fontFamily: 'var(--font-heading)' }}>
          Администрирование
        </h1>
      </div>

      {error && (
        <div className="card p-4 mb-4" role="alert" data-testid="admin-error">
          <span className="text-[13px]">{error}</span>
        </div>
      )}

      {adminCount === 1 && (
        <div className="card p-4 mb-4" data-testid="last-admin-notice">
          <span className="text-[13px] muted">
            Администратор в системе один. Пока это так, его нельзя отключить или лишить прав —
            иначе управление доступом будет потеряно безвозвратно.
          </span>
        </div>
      )}

      <section className="card p-4 mb-4" aria-label="Роли">
        <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
          Роли
        </h2>
        {roles.isLoading && (
          <p className="muted text-[13px]" role="status" data-testid="roles-loading">
            Загрузка…
          </p>
        )}
        {roles.isError && (
          <p className="text-[13px]" role="alert" data-testid="roles-error">
            Не удалось загрузить роли.
          </p>
        )}
        {!roles.isLoading && !roles.isError && roleList.length === 0 && (
          <p className="muted text-[13px]" data-testid="roles-empty">
            Ролей пока нет.
          </p>
        )}

        <ul className="space-y-2">
          {roleList.map((role) => (
            <li key={role.id} data-testid="role-row">
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  className="text-[14px] font-medium"
                  aria-expanded={openRole === role.id}
                  onClick={() => setOpenRole(openRole === role.id ? null : role.id)}
                >
                  {role.name}
                </button>
                <span className="muted text-[12px]">
                  {role.permissions.length} прав · {role.user_count} польз.
                </span>
              </div>

              {openRole === role.id && catalogue.data && (
                <PermissionGrid
                  role={role}
                  resources={catalogue.data.resources}
                  disabled={!mayManage || setPermissions.isPending}
                  onChange={(permissions) =>
                    run(() => setPermissions.mutateAsync({ roleId: role.id, permissions }))
                  }
                />
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="card p-4" aria-label="Пользователи">
        <h2 className="text-[15px] mb-3" style={{ fontFamily: 'var(--font-heading)' }}>
          Пользователи
        </h2>
        {users.isLoading && (
          <p className="muted text-[13px]" role="status" data-testid="users-loading">
            Загрузка…
          </p>
        )}
        {users.isError && (
          <p className="text-[13px]" role="alert" data-testid="users-error">
            Не удалось загрузить пользователей.
          </p>
        )}
        {!users.isLoading && !users.isError && userList.length === 0 && (
          <p className="muted text-[13px]" data-testid="users-empty">
            Пользователей нет.
          </p>
        )}

        {userList.length > 0 && (
          <table className="table w-full">
            <thead>
              <tr>
                <th>Пользователь</th>
                <th>Роли</th>
                <th>Статус</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {userList.map((user) => (
                <tr key={user.id} data-testid="user-row">
                  <td>
                    <div className="text-[13.5px]">{user.full_name}</div>
                    <div className="muted text-[12px]">{user.email}</div>
                  </td>
                  <td>
                    <RolePicker
                      user={user}
                      roles={roleList}
                      disabled={!mayManage || setUserRoles.isPending}
                      onChange={(roleIds) =>
                        run(() => setUserRoles.mutateAsync({ userId: user.id, roleIds }))
                      }
                    />
                  </td>
                  <td>
                    <StatusTag status={user.status} />
                  </td>
                  <td className="text-right">
                    {mayManage && (
                      <button
                        type="button"
                        className="btn btn-secondary"
                        disabled={setActive.isPending}
                        onClick={() =>
                          run(() =>
                            setActive.mutateAsync({ userId: user.id, active: !user.is_active }),
                          )
                        }
                      >
                        {user.is_active ? 'Отключить' : 'Включить'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </AppShell>
  );
}

/**
 * The permission grid.
 *
 * Generated from the catalogue the server sends, which is built from rbac's own
 * tables — so the editor cannot offer a permission the middleware does not
 * recognise, and a new module's permissions appear here the moment they are
 * declared.
 */
function PermissionGrid({
  role,
  resources,
  disabled,
  onChange,
}: {
  role: RoleDetail;
  resources: { key: string; actions: string[] }[];
  disabled: boolean;
  onChange: (permissions: string[]) => void;
}) {
  const held = new Set(role.permissions);

  function toggle(permission: string) {
    const next = new Set(held);
    if (next.has(permission)) next.delete(permission);
    else next.add(permission);
    onChange([...next]);
  }

  return (
    <div className="mt-3 overflow-x-auto" data-testid="permission-grid">
      <table className="table w-full text-[12.5px]">
        <thead>
          <tr>
            <th>Модуль</th>
            {resources[0]?.actions.map((a) => (
              <th key={a} className="text-center">
                {actionLabel(a)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {resources.map((resource) => (
            <tr key={resource.key}>
              <td>{resource.key}</td>
              {resource.actions.map((action) => {
                const permission = `${resource.key}:${action}`;
                return (
                  <td key={action} className="text-center">
                    <input
                      type="checkbox"
                      checked={held.has(permission)}
                      disabled={disabled}
                      aria-label={permission}
                      onChange={() => toggle(permission)}
                    />
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RolePicker({
  user,
  roles,
  disabled,
  onChange,
}: {
  user: AdminUserRow;
  roles: RoleDetail[];
  disabled: boolean;
  onChange: (roleIds: string[]) => void;
}) {
  const held = new Set(user.roles);

  return (
    <div className="flex flex-wrap gap-2">
      {roles.map((role) => (
        <label key={role.id} className="flex items-center gap-1.5 text-[12.5px]">
          <input
            type="checkbox"
            checked={held.has(role.key)}
            disabled={disabled}
            aria-label={`${user.email}: ${role.key}`}
            onChange={() => {
              const next = new Set(held);
              if (next.has(role.key)) next.delete(role.key);
              else next.add(role.key);
              // The API takes ids; the row carries keys, so they are mapped back.
              onChange(roles.filter((r) => next.has(r.key)).map((r) => r.id));
            }}
          />
          {role.name}
        </label>
      ))}
      {roles.length === 0 && <span className="muted text-[12px]">Ролей нет</span>}
    </div>
  );
}

function actionLabel(action: string): string {
  const labels: Record<string, string> = {
    read: 'Чтение',
    manage: 'Изменение',
    approve: 'Согласование',
  };
  return labels[action] ?? action;
}

/** Whether a user holds admin:manage through any of their roles. */
function holdsAdmin(user: AdminUserRow, roles: RoleDetail[]): boolean {
  return roles.some(
    (role) => user.roles.includes(role.key) && role.permissions.includes('admin:manage'),
  );
}
