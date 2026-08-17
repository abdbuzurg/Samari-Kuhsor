'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { PageMeta } from '@samari/types';

import { ApiError } from '@/lib/session';

/**
 * The shared data layer for a CRUD module.
 *
 * docs/07-IMPLEMENTATION-PLAN.md I2: build the reference slice concrete, then
 * EXTRACT the engine from working code rather than designing it against a spec.
 * This is that extraction — every line here was written first for Товары and is
 * kept only because a second module would otherwise copy it verbatim.
 *
 * What is deliberately NOT abstracted:
 *   - The columns, KPIs and field groups. Those differ per module by design and
 *     come from the approved prototype; a config DSL for them would be a worse
 *     way to write JSX.
 *   - Validation and business rules. Those live in Go (CLAUDE.md §3).
 *   - Anything whose only second consumer is hypothetical.
 *
 * Every request goes to /api/* on this origin. The browser never learns the Go
 * API's address (CLAUDE.md §3).
 */

export interface Collection<T> {
  data: T[];
  meta: PageMeta;
}

/** Query parameters common to every module list (docs/03-API-CONTRACT.md §5). */
export interface ListQuery {
  q?: string;
  page?: number;
  sort?: string;
  locale?: string;
  /** Module-specific filters, e.g. status or item_type. */
  filters?: Record<string, string | undefined>;
}

export function toSearchParams(query: ListQuery): string {
  const params = new URLSearchParams();
  if (query.q) params.set('q', query.q);
  if (query.page && query.page > 1) params.set('page', String(query.page));
  if (query.sort) params.set('sort', query.sort);
  if (query.locale) params.set('locale', query.locale);
  for (const [key, value] of Object.entries(query.filters ?? {})) {
    if (value) params.set(key, value);
  }
  const s = params.toString();
  return s ? `?${s}` : '';
}

async function parse(res: Response): Promise<{ data?: unknown; meta?: PageMeta }> {
  if (res.status === 204) return {};
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    // Switch on the stable code, never the message (docs/03-API-CONTRACT.md:116).
    throw new ApiError(
      res.status,
      body?.error?.code ?? 'internal_error',
      body?.error?.message ?? '',
      body?.error?.details,
    );
  }
  return body ?? {};
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  return (await parse(res)).data as T;
}

/**
 * Builds the hooks for one resource.
 *
 * `resource` is both the URL segment and the cache-key root, so the two can never
 * disagree — a mismatch there produces stale lists after a save, which is the
 * kind of bug that looks like "sometimes it doesn't refresh".
 */
export function createResourceHooks<Row, Detail extends { id: string; version: number }>(
  resource: string,
) {
  const listKey = [resource, 'list'] as const;
  const oneKey = (id: string | undefined) => [resource, 'one', id] as const;

  function useList(query: ListQuery) {
    return useQuery<Collection<Row>>({
      // Every parameter is in the key, so typing in the search box refetches
      // rather than serving the previous term's results.
      queryKey: [...listKey, query],
      queryFn: async () => {
        const res = await fetch(`/api/${resource}${toSearchParams(query)}`, {
          headers: { 'Content-Type': 'application/json' },
        });
        const body = await parse(res);
        return { data: (body.data ?? []) as Row[], meta: body.meta as PageMeta };
      },
      // Keeps the previous page visible while the next loads, so the table does
      // not flash empty on every keystroke.
      placeholderData: (previous) => previous,
    });
  }

  function useOne(id: string | undefined) {
    return useQuery<Detail>({
      queryKey: oneKey(id),
      queryFn: () => send<Detail>(`/api/${resource}/${id}`),
      enabled: !!id,
    });
  }

  function useCreate() {
    const qc = useQueryClient();
    return useMutation({
      mutationFn: (body: unknown) =>
        send<Detail>(`/api/${resource}`, { method: 'POST', body: JSON.stringify(body) }),
      onSuccess: () => qc.invalidateQueries({ queryKey: listKey }),
    });
  }

  function useUpdate(id: string) {
    const qc = useQueryClient();
    return useMutation({
      mutationFn: (body: unknown) =>
        send<Detail>(`/api/${resource}/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
      onSuccess: (updated) => {
        // Seed the detail cache from the response so the version the NEXT edit
        // sends is the one the server just wrote. Invalidating alone leaves a
        // window where the form still holds the stale version and 409s against
        // its own previous save.
        qc.setQueryData(oneKey(id), updated);
        qc.invalidateQueries({ queryKey: listKey });
      },
    });
  }

  function useRemove(id: string) {
    const qc = useQueryClient();
    return useMutation({
      // A tombstone carries a version for the same reason an edit does: deleting
      // a record someone else just changed must fail the same way.
      mutationFn: (version: number) =>
        send<void>(`/api/${resource}/${id}`, {
          method: 'DELETE',
          body: JSON.stringify({ version }),
        }),
      onSuccess: () => {
        qc.removeQueries({ queryKey: oneKey(id) });
        qc.invalidateQueries({ queryKey: listKey });
      },
    });
  }

  /** POST to a sub-resource — the shape state transitions use
   *  (docs/03-API-CONTRACT.md:74). */
  function useAction<Result = unknown>(id: string, action: string) {
    const qc = useQueryClient();
    return useMutation({
      mutationFn: (body?: unknown) =>
        send<Result>(`/api/${resource}/${id}/${action}`, {
          method: 'POST',
          body: body === undefined ? undefined : JSON.stringify(body),
        }),
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: oneKey(id) });
        qc.invalidateQueries({ queryKey: listKey });
      },
    });
  }

  return { useList, useOne, useCreate, useUpdate, useRemove, useAction };
}

/**
 * The placeholder for a value the client has not yet verified.
 *
 * docs/02-SCHEMA.md:176 — compositions, nutritional values, shelf life and water
 * classification stay null until the recipes are approved and lab-tested. The
 * client set that rule explicitly. An empty cell reads as "none"; «уточняется»
 * reads as "not yet determined", which is the truth.
 */
export const TBC = 'уточняется';

export function orTBC(value: string | number | null | undefined): string {
  return value === null || value === undefined || value === '' ? TBC : String(value);
}
