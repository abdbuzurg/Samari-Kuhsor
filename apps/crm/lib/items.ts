'use client';

import type { Item, ItemListRow } from '@samari/types';

import { createResourceHooks, type ListQuery } from '@/lib/resource';

/**
 * Товары и цены — the module's data layer.
 *
 * After the T15 extraction this file is what a module actually costs: a resource
 * name, its two row types, and whatever is genuinely specific to it. Everything
 * else — fetching, cache keys, the version-conflict-safe update, the 204 on
 * delete — comes from createResourceHooks (docs/07-IMPLEMENTATION-PLAN.md I2).
 *
 * Склад is the next consumer, and its version of this file should be about this
 * long.
 */

const items = createResourceHooks<ItemListRow, Item>('items');

export interface ItemQuery {
  q?: string;
  status?: string;
  itemType?: string;
  page?: number;
  sort?: string;
  locale?: string;
}

/** Maps the module's own filter names onto the generic query shape. */
function toListQuery(query: ItemQuery): ListQuery {
  return {
    q: query.q,
    page: query.page,
    sort: query.sort,
    locale: query.locale,
    filters: { status: query.status, item_type: query.itemType },
  };
}

export function useItems(query: ItemQuery) {
  return items.useList(toListQuery(query));
}

export const useItem = items.useOne;
export const useCreateItem = items.useCreate;
export const useUpdateItem = items.useUpdate;
export const useDeleteItem = items.useRemove;

/** Prices are a sub-resource: a new price supersedes the open one rather than
 *  replacing it, because the history records what a product cost when an order
 *  was placed. */
export function useAddPrice(id: string) {
  return items.useAction(id, 'prices');
}

export { TBC, orTBC } from '@/lib/resource';
export type { Collection } from '@/lib/resource';
