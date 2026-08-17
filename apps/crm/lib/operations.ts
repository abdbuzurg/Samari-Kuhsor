'use client';

import type {
  AdminUserRow,
  ContentBlock,
  ContentPage,
  Asset,
  AuditEntry,
  BatchDetail,
  BatchListRow,
  Document,
  Employee,
  Inquiry,
  MaintenanceEvent,
  ManufacturingOrder,
  ManufacturingOrderRow,
  MediaItem,
  NewsPost,
  NewsTranslation,
  PermissionCatalogue,
  PurchaseOrder,
  RoleDetail,
  PurchaseOrderRow,
  SalesOrder,
  SalesOrderRow,
  Shipment,
  StockBalanceRow,
  Supplier,
} from '@samari/types';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/session';
import {
  createResourceHooks,
  toSearchParams,
  type Collection,
  type ListQuery,
} from '@/lib/resource';

/**
 * The data layer for the operating chain and commerce modules.
 *
 * This file is the return on the T15 extraction. Each module costs a
 * createResourceHooks call, its two row types, and a filter mapping — the
 * fetching, cache keys, version-safe update and 204-on-delete all come from the
 * engine (docs/07-IMPLEMENTATION-PLAN.md I2).
 *
 * They share a file rather than getting one each because none of them has
 * anything module-specific left to say. The moment one does, it moves out.
 */

/** A status filter is the only thing every one of these lists needs. */
export interface StatusQuery {
  q?: string;
  status?: string;
  page?: number;
  sort?: string;
  locale?: string;
}

function withStatus(query: StatusQuery): ListQuery {
  return {
    q: query.q,
    page: query.page,
    sort: query.sort,
    locale: query.locale,
    filters: { status: query.status },
  };
}

// ---------------------------------------------------------------------------
// Склад и запасы
// ---------------------------------------------------------------------------

const stock = createResourceHooks<StockBalanceRow, never>('stock');

export interface StockQuery {
  q?: string;
  zone?: string;
  itemId?: string;
  page?: number;
  sort?: string;
}

export function useStock(query: StockQuery) {
  return stock.useList({
    q: query.q,
    page: query.page,
    sort: query.sort,
    filters: { zone: query.zone, item_id: query.itemId },
  });
}

// ---------------------------------------------------------------------------
// Производство
// ---------------------------------------------------------------------------

const manufacturing = createResourceHooks<ManufacturingOrderRow, ManufacturingOrder>(
  'manufacturing-orders',
);

export function useManufacturingOrders(query: StatusQuery) {
  return manufacturing.useList(withStatus(query));
}
export const useManufacturingOrder = manufacturing.useOne;
export const useCreateManufacturingOrder = manufacturing.useCreate;

/** Records a shift entry. Append-only — see the domain comment on RecordEntry. */
export function useRecordEntry(id: string) {
  return manufacturing.useAction(id, 'entries');
}

/** Completes the order: posts the output to quarantine and moves the batch. */
export function useCompleteOrder(id: string) {
  return manufacturing.useAction(id, 'complete');
}

// ---------------------------------------------------------------------------
// Качество и безопасность
// ---------------------------------------------------------------------------

// Only the list comes from this resource. The batch detail is a different
// endpoint under a different module (/api/batches/{id}/detail), because
// traceability joins tests, decision history and stock — it is not the list row
// with more fields, and BatchDetail has no top-level id or version to satisfy
// the engine's detail constraint.
const batches = createResourceHooks<BatchListRow, BatchListRow>('quality/batches');

export function useQualityBatches(query: StatusQuery) {
  return batches.useList(withStatus(query));
}

/** The traceability view: batch, tests, decision history and where its stock is. */
export function useBatchDetail(id: string | undefined) {
  return useQuery<BatchDetail>({
    queryKey: ['batch-detail', id],
    queryFn: async () => {
      const res = await fetch(`/api/batches/${id}/detail`, {
        headers: { 'Content-Type': 'application/json' },
      });
      const body = await res.json();
      if (!res.ok) {
        throw new ApiError(
          res.status,
          body?.error?.code ?? 'internal_error',
          body?.error?.message ?? '',
        );
      }
      return body.data as BatchDetail;
    },
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Закупки
// ---------------------------------------------------------------------------

const suppliers = createResourceHooks<Supplier, Supplier>('suppliers');
export function useSuppliers(query: StatusQuery) {
  return suppliers.useList(withStatus(query));
}
export const useCreateSupplier = suppliers.useCreate;

const purchaseOrders = createResourceHooks<PurchaseOrderRow, PurchaseOrder>('purchase-orders');
export function usePurchaseOrders(query: StatusQuery) {
  return purchaseOrders.useList(withStatus(query));
}
export const usePurchaseOrder = purchaseOrders.useOne;
export const useCreatePurchaseOrder = purchaseOrders.useCreate;
export function useTransitionPurchaseOrder(id: string) {
  return purchaseOrders.useAction<PurchaseOrderRow>(id, 'transition');
}
export function useReceivePurchaseOrder(id: string) {
  return purchaseOrders.useAction(id, 'receipts');
}

// ---------------------------------------------------------------------------
// Продажи
// ---------------------------------------------------------------------------

const salesOrders = createResourceHooks<SalesOrderRow, SalesOrder>('sales-orders');
export function useSalesOrders(query: StatusQuery) {
  return salesOrders.useList(withStatus(query));
}
export const useSalesOrder = salesOrders.useOne;
export const useCreateSalesOrder = salesOrders.useCreate;

/** Confirming is the gate: Go checks every line's batch is released. */
export function useConfirmSalesOrder(id: string) {
  return salesOrders.useAction<SalesOrderRow>(id, 'confirm');
}

// ---------------------------------------------------------------------------
// Логистика
// ---------------------------------------------------------------------------

const shipments = createResourceHooks<Shipment, Shipment>('shipments');
export function useShipments(query: StatusQuery) {
  return shipments.useList(withStatus(query));
}
export const useShipment = shipments.useOne;
export const useCreateShipment = shipments.useCreate;
export function useLoadShipment(id: string) {
  return shipments.useAction(id, 'lines');
}

// ---------------------------------------------------------------------------
// Интеграция с сайтом
// ---------------------------------------------------------------------------

const inquiries = createResourceHooks<Inquiry, Inquiry>('inquiries');

export interface InquiryQuery extends StatusQuery {
  type?: string;
}

export function useInquiries(query: InquiryQuery) {
  return inquiries.useList({
    q: query.q,
    page: query.page,
    sort: query.sort,
    filters: { status: query.status, type: query.type },
  });
}

export function useConvertInquiry(id: string) {
  return inquiries.useAction(id, 'convert');
}

// ---------------------------------------------------------------------------
// Персонал, Оборудование и ТО, Документы
// ---------------------------------------------------------------------------

const employees = createResourceHooks<Employee, Employee>('employees');
export function useEmployees(query: StatusQuery) {
  return employees.useList(withStatus(query));
}
export const useCreateEmployee = employees.useCreate;
export const useUpdateEmployee = employees.useUpdate;

const assets = createResourceHooks<Asset, Asset>('assets');
export function useAssets(query: StatusQuery) {
  return assets.useList(withStatus(query));
}
export const useCreateAsset = assets.useCreate;

/** Records a service. Go clears the maintenance-due flag as part of it. */
export function useRecordMaintenance(id: string) {
  return assets.useAction<MaintenanceEvent>(id, 'maintenance');
}

const documents = createResourceHooks<Document, Document>('documents');
export function useDocuments(query: StatusQuery) {
  return documents.useList(withStatus(query));
}
export const useCreateDocument = documents.useCreate;
export function useTransitionDocument(id: string) {
  return documents.useAction<Document>(id, 'transition');
}

// ---------------------------------------------------------------------------
// Администрирование
// ---------------------------------------------------------------------------

const roles = createResourceHooks<RoleDetail, RoleDetail>('admin/roles');
export function useRoles() {
  return roles.useList({});
}
export const useCreateRole = roles.useCreate;
export const useDeleteRole = roles.useRemove;

const users = createResourceHooks<AdminUserRow, AdminUserRow>('admin/users');
export function useAdminUsers(query: StatusQuery) {
  return users.useList(withStatus(query));
}

/** The permission catalogue the role editor renders its checkboxes from. */
export function usePermissionCatalogue() {
  return useQuery<PermissionCatalogue>({
    queryKey: ['admin', 'permissions'],
    queryFn: () => getJSON<PermissionCatalogue>('/api/admin/permissions'),
  });
}

/** PUT, not POST: replacing a role's whole permission set is idempotent, and a
 *  partial update would make "revoke" impossible to express. */
export function useSetRolePermissions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ roleId, permissions }: { roleId: string; permissions: string[] }) =>
      putJSON(`/api/admin/roles/${roleId}/permissions`, { permissions }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin/roles'] }),
  });
}

export function useSetUserRoles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, roleIds }: { userId: string; roleIds: string[] }) =>
      putJSON(`/api/admin/users/${userId}/roles`, { role_ids: roleIds }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin/users'] }),
  });
}

export function useSetUserActive() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, active }: { userId: string; active: boolean }) =>
      putJSON(`/api/admin/users/${userId}/active`, { active }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin/users'] }),
  });
}

export interface AuditQuery {
  resource?: string;
  actorId?: string;
  page?: number;
}

/**
 * The audit log, read with a plain query rather than through the resource engine.
 *
 * The engine's detail type requires `version`, and an audit entry has none —
 * deliberately. It is append-only evidence with no update path, so there is
 * nothing to guard a concurrent edit against. Forcing it through the CRUD engine
 * would mean inventing a field to satisfy a constraint that does not apply.
 */
export function useAuditLog(query: AuditQuery) {
  return useQuery<Collection<AuditEntry>>({
    queryKey: ['audit', query],
    queryFn: async () => {
      const search = toSearchParams({
        page: query.page,
        filters: { resource: query.resource, actor_id: query.actorId },
      });
      const res = await fetch(`/api/audit${search}`, {
        headers: { 'Content-Type': 'application/json' },
      });
      const body = await res.json();
      if (!res.ok) {
        throw new ApiError(
          res.status,
          body?.error?.code ?? 'internal_error',
          body?.error?.message ?? '',
        );
      }
      return { data: body.data ?? [], meta: body.meta };
    },
    placeholderData: (previous) => previous,
  });
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' } });
  const body = await res.json();
  if (!res.ok) {
    throw new ApiError(res.status, body?.error?.code ?? 'internal_error', body?.error?.message ?? '');
  }
  return body.data as T;
}

async function putJSON(path: string, payload: unknown): Promise<void> {
  const res = await fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      body?.error?.code ?? 'internal_error',
      body?.error?.message ?? '',
      body?.error?.details,
    );
  }
}

// ---------------------------------------------------------------------------
// CMS
// ---------------------------------------------------------------------------

/** Pages are four fixed keys, not a growing list — no paging. */
export function useContentPages() {
  return useQuery<ContentPage[]>({
    queryKey: ['cms', 'pages'],
    queryFn: () => getJSON<ContentPage[]>('/api/cms/pages'),
  });
}

export function useContentBlocks(pageId: string | undefined, locale: string) {
  return useQuery<ContentBlock[]>({
    queryKey: ['cms', 'blocks', pageId, locale],
    queryFn: () => getJSON<ContentBlock[]>(`/api/cms/pages/${pageId}/blocks?locale=${locale}`),
    enabled: !!pageId,
  });
}

export function useSaveContentBlock(pageId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) => putJSON(`/api/cms/pages/${pageId}/blocks`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cms', 'blocks', pageId] }),
  });
}

/**
 * Moves content along the ladder.
 *
 * Invalidates the WHOLE cms cache, not just the entity: a transition changes
 * what the list shows, what the history shows, and whether the editor is
 * writable. Invalidating narrowly here is how a screen ends up showing a
 * published page with an enabled edit form.
 */
export function useCMSTransition(kind: 'pages' | 'news', id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { to: string; comment?: string }) =>
      postJSON(`/api/cms/${kind}/${id}/transition`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cms'] }),
  });
}

export function useNewsPosts(query: StatusQuery) {
  return useQuery<Collection<NewsPost>>({
    queryKey: ['cms', 'news', query],
    queryFn: async () => {
      const search = toSearchParams({
        q: query.q,
        page: query.page,
        filters: { status: query.status },
      });
      const res = await fetch(`/api/cms/news${search}`, {
        headers: { 'Content-Type': 'application/json' },
      });
      const body = await res.json();
      if (!res.ok) {
        throw new ApiError(
          res.status,
          body?.error?.code ?? 'internal_error',
          body?.error?.message ?? '',
        );
      }
      return { data: body.data ?? [], meta: body.meta };
    },
    placeholderData: (previous) => previous,
  });
}

export function useCreateNewsPost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) => postJSON('/api/cms/news', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cms'] }),
  });
}

export function useNewsTranslations(postId: string | undefined) {
  return useQuery<NewsTranslation[]>({
    queryKey: ['cms', 'news-translations', postId],
    queryFn: () => getJSON<NewsTranslation[]>(`/api/cms/news/${postId}/translations`),
    enabled: !!postId,
  });
}

export function useSaveNewsTranslation(postId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: unknown) => putJSON(`/api/cms/news/${postId}/translations`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cms'] }),
  });
}

export function useMediaLibrary(query: StatusQuery) {
  return useQuery<Collection<MediaItem>>({
    queryKey: ['cms', 'media', query],
    queryFn: async () => {
      const res = await fetch(`/api/cms/media${toSearchParams({ q: query.q, page: query.page })}`, {
        headers: { 'Content-Type': 'application/json' },
      });
      const body = await res.json();
      if (!res.ok) {
        throw new ApiError(
          res.status,
          body?.error?.code ?? 'internal_error',
          body?.error?.message ?? '',
        );
      }
      return { data: body.data ?? [], meta: body.meta };
    },
  });
}

async function postJSON(path: string, payload: unknown): Promise<void> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      body?.error?.code ?? 'internal_error',
      body?.error?.message ?? '',
      body?.error?.details,
    );
  }
}
