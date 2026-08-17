'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Item, ItemListRow, PageMeta } from '@samari/types';

import { ApiError } from '@/lib/session';

/**
 * Товары и цены data hooks.
 *
 * Everything goes through /api/* on this origin; the browser never learns the Go
 * API's address (CLAUDE.md §3). These hooks are what T15 generalises into the
 * shared list/detail engine, so the shape here is deliberately module-agnostic
 * apart from the types.
 */

export interface Collection<T> {
  data: T[];
  meta: PageMeta;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  if (res.status === 204) return undefined as T;

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
  return body?.data as T;
}

/** The collection response keeps `meta` alongside `data`, so fetch it whole. */
async function requestCollection<T>(path: string): Promise<Collection<T>> {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' } });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(
      res.status,
      body?.error?.code ?? 'internal_error',
      body?.error?.message ?? '',
      body?.error?.details,
    );
  }
  return { data: body?.data ?? [], meta: body?.meta };
}

export interface ItemQuery {
  q?: string;
  status?: string;
  itemType?: string;
  page?: number;
  sort?: string;
  locale?: string;
}

function toSearch(query: ItemQuery): string {
  const params = new URLSearchParams();
  if (query.q) params.set('q', query.q);
  if (query.status) params.set('status', query.status);
  if (query.itemType) params.set('item_type', query.itemType);
  if (query.page && query.page > 1) params.set('page', String(query.page));
  if (query.sort) params.set('sort', query.sort);
  if (query.locale) params.set('locale', query.locale);
  const s = params.toString();
  return s ? `?${s}` : '';
}

export function useItems(query: ItemQuery) {
  return useQuery<Collection<ItemListRow>>({
    // The query key includes every parameter, so typing in the search box
    // refetches rather than serving the previous term's results.
    queryKey: ['items', query],
    queryFn: () => requestCollection<ItemListRow>(`/api/items${toSearch(query)}`),
    // Keeps the previous page visible while the next loads, so the table does
    // not flash empty on every keystroke.
    placeholderData: (previous) => previous,
  });
}

export function useItem(id: string | undefined) {
  return useQuery<Item>({
    queryKey: ['item', id],
    queryFn: () => request<Item>(`/api/items/${id}`),
    enabled: !!id,
  });
}

export function useCreateItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) =>
      request<Item>('/api/items', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['items'] }),
  });
}

export function useUpdateItem(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) =>
      request<Item>(`/api/items/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
    onSuccess: (updated) => {
      // Seed the detail cache from the response so the version the next edit
      // sends is the one the server just wrote. Invalidating alone would leave a
      // window where the form still holds the stale version and 409s.
      qc.setQueryData(['item', id], updated);
      qc.invalidateQueries({ queryKey: ['items'] });
    },
  });
}

export function useDeleteItem(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: number) =>
      request<void>(`/api/items/${id}`, {
        method: 'DELETE',
        body: JSON.stringify({ version }),
      }),
    onSuccess: () => {
      qc.removeQueries({ queryKey: ['item', id] });
      qc.invalidateQueries({ queryKey: ['items'] });
    },
  });
}

export function useAddPrice(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) =>
      request(`/api/items/${id}/prices`, { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['item', id] });
      qc.invalidateQueries({ queryKey: ['items'] });
    },
  });
}

/**
 * The placeholder for a value the client has not yet verified.
 *
 * docs/02-SCHEMA.md:176 — compositions, nutritional values, shelf life and water
 * classification stay null until the recipes are approved and lab-tested. The
 * client set that rule explicitly: the system must not publish unverified claims.
 * Rendering an empty cell would read as "no preservatives" rather than "unknown".
 */
export const TBC = 'уточняется';

/** Renders a value or the «уточняется» placeholder. */
export function orTBC(value: string | null | undefined): string {
  return value === null || value === undefined || value === '' ? TBC : value;
}
