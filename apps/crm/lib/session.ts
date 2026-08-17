'use client';

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { User } from '@samari/types';

/**
 * Client-side session access.
 *
 * Everything goes through /api/* on this origin. The browser never learns the Go
 * API's address (CLAUDE.md §3), and the session token lives in an httpOnly
 * cookie it cannot read.
 */

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });

  if (res.status === 204) return undefined as T;

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    const error = body?.error;
    // Switch on the stable code, never the message (docs/03-API-CONTRACT.md:116).
    throw new ApiError(res.status, error?.code ?? 'internal_error', error?.message ?? '');
  }
  return body?.data as T;
}

export function useSession() {
  return useQuery<User>({
    queryKey: ['session'],
    queryFn: () => request<User>('/api/auth/me'),
    retry: false,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (credentials: { email: string; password: string }) =>
      request<{ user: User }>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify(credentials),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['session'] }),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => request<void>('/api/auth/logout', { method: 'POST' }),
    onSuccess: () => qc.clear(),
  });
}

/**
 * Permission check for hiding UI.
 *
 * `manage` implies `read`; `approve` implies nothing (docs/04-RBAC.md:116).
 * This is presentation only — a hidden button is never a control.
 */
export function can(
  permissions: readonly string[] | undefined,
  resource: string,
  action: 'read' | 'manage' | 'approve',
): boolean {
  if (!permissions) return false;
  if (permissions.includes(`${resource}:${action}`)) return true;
  if (action === 'read') return permissions.includes(`${resource}:manage`);
  return false;
}
