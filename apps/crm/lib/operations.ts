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
  CRMKPIs,
  Customer,
  CustomerDetail,
  CustomerRow,
  Deal,
  DealRow,
  Location,
  StockBalanceRow,
  Task,
  StockMovementRow,
  Supplier,
  WorkflowEvent,
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

/**
 * One position's movement ledger, with the running balance after each row.
 *
 * This is the answer to "why does it say 480?", and it is the reason no balance
 * is ever stored (CLAUDE.md §4.2): every figure on the Склад screen can be
 * explained by opening this.
 */
export function useStockLedger(query: {
  itemId?: string;
  batchId?: string | null;
  locationId?: string;
  page?: number;
}) {
  return useQuery<Collection<StockMovementRow>>({
    queryKey: ['stock-ledger', query],
    queryFn: async () => {
      const search = toSearchParams({
        page: query.page,
        filters: {
          item_id: query.itemId,
          batch_id: query.batchId ?? undefined,
          location_id: query.locationId,
        },
      });
      const res = await fetch(`/api/stock/ledger${search}`, {
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
    enabled: !!query.itemId && !!query.locationId,
  });
}

/**
 * The catalogue, for a picker.
 *
 * A create form needs every product, not one page of them — the catalogue is
 * exactly five (D8), so a single unpaged read is the whole list.
 */
export function useItemsForPicker() {
  return useQuery<Array<{ id: string; sku: string; name: string }>>({
    queryKey: ['items', 'picker'],
    queryFn: async () => {
      const res = await fetch('/api/items?per_page=100', {
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
      return (body.data ?? []) as Array<{ id: string; sku: string; name: string }>;
    },
  });
}

/** The warehouse's locations — needed by every movement and transfer form. */
export function useLocations() {
  return useQuery<Location[]>({
    queryKey: ['locations'],
    queryFn: async () => {
      const res = await fetch('/api/locations', {
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
      return (body.data ?? []) as Location[];
    },
  });
}

/**
 * Posts one movement.
 *
 * NO endpoint anywhere accepts an absolute quantity, and no form may offer one
 * (05-MODULES.md:112). This takes a DELTA. A correction is a compensating entry,
 * never an edit of the row that was wrong.
 */
export function usePostMovement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      item_id: string;
      batch_id?: string;
      location_id: string;
      qty_delta: string;
      reason: string;
      note?: string;
    }) => postJSON('/api/stock/movements', body),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ['stock'] }),
        qc.invalidateQueries({ queryKey: ['stock-ledger'] }),
      ]),
  });
}

/** A transfer is TWO rows sharing a ref_id, netting to zero. Go writes both. */
export function useTransferStock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      item_id: string;
      batch_id?: string;
      from_location_id: string;
      to_location_id: string;
      qty: string;
      note?: string;
    }) => postJSON('/api/stock/transfers', body),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ['stock'] }),
        qc.invalidateQueries({ queryKey: ['stock-ledger'] }),
      ]),
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

/**
 * Creates a batch.
 *
 * Needed BEFORE the plant produces anything: QR payloads are printed onto
 * wrappers ordered months in advance (D11), so a batch number has to exist
 * long before there is product to put in it.
 */
export function useCreateBatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      batch_no: string;
      item_id: string;
      produced_on?: string;
      expires_on?: string;
    }) => postJSON<{ id: string }>('/api/batches', body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['quality/batches'] }),
  });
}

/** Issues the batch's QR payload. Once issued it never changes (D11). */
export function useIssueBatchQR(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON(`/api/batches/${id}/qr`, {}),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ['batch-detail'] }),
        qc.invalidateQueries({ queryKey: ['quality/batches'] }),
      ]),
  });
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
//
// Ladder transitions go through the generic `useTransition` behind
// WorkflowActions, so there is no per-module transition hook here. Three of them
// existed and were superseded in R01; they were removed rather than left as a
// second way to do the same thing.
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
export function useReceivePurchaseOrder(id: string) {
  return purchaseOrders.useAction(id, 'receipts');
}

// ---------------------------------------------------------------------------
// CRM и продажи — the customer side (R12/R13)
// ---------------------------------------------------------------------------

const customers = createResourceHooks<CustomerRow, Customer>('customers');
export function useCustomers(query: StatusQuery & { region?: string }) {
  return customers.useList({
    q: query.q,
    page: query.page,
    sort: query.sort,
    filters: { region: query.region },
  });
}
export const useCreateCustomer = customers.useCreate;
export const useUpdateCustomer = customers.useUpdate;

/** The customer detail: header plus every band (docs/05-MODULES.md:179). */
export function useCustomerDetail(id: string | undefined) {
  return useQuery<CustomerDetail>({
    queryKey: ['customer-detail', id],
    queryFn: () => getJSON<CustomerDetail>(`/api/customers/${id}`),
    enabled: !!id,
  });
}

export function useCreateContact(customerId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { full_name: string; role?: string; email?: string; phone?: string }) =>
      postJSON(`/api/customers/${customerId}/contacts`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['customer-detail'] }),
  });
}

const deals = createResourceHooks<DealRow, Deal>('deals');
export function useDeals(query: StatusQuery & { stage?: string; customerId?: string }) {
  return deals.useList({
    q: query.q,
    page: query.page,
    sort: query.sort,
    filters: { stage: query.stage, customer_id: query.customerId },
  });
}
export const useDeal = deals.useOne;
export const useCreateDeal = deals.useCreate;


const tasks = createResourceHooks<Task, Task>('tasks');
export function useTasks(query: StatusQuery) {
  return tasks.useList(withStatus(query));
}
export const useCreateTask = tasks.useCreate;

export function useSetTaskStatus(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { status: string }) => putJSON(`/api/tasks/${id}/status`, body),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ['tasks'] }),
        qc.invalidateQueries({ queryKey: ['crm-kpis'] }),
      ]),
  });
}

/** Новые лиды · Открытые сделки · Конверсия · Просроченные задачи. */
export function useCRMKPIs() {
  return useQuery<CRMKPIs>({
    queryKey: ['crm-kpis'],
    queryFn: () => getJSON<CRMKPIs>('/api/crm/kpis'),
  });
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
export const useInquiry = inquiries.useOne;

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
export const useEmployee = employees.useOne;
export function useEmployees(query: StatusQuery) {
  return employees.useList(withStatus(query));
}
export const useCreateEmployee = employees.useCreate;
export const useUpdateEmployee = employees.useUpdate;

const assets = createResourceHooks<Asset, Asset>('assets');
export const useAsset = assets.useOne;
export function useAssets(query: StatusQuery) {
  return assets.useList(withStatus(query));
}
export const useCreateAsset = assets.useCreate;

/** An asset's service history, newest first. */
export function useAssetMaintenance(id: string | undefined) {
  return useQuery<MaintenanceEvent[]>({
    queryKey: ['assets', 'maintenance', id],
    queryFn: async () => {
      const res = await fetch(`/api/assets/${id}/maintenance`, {
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
      return (body.data ?? []) as MaintenanceEvent[];
    },
    enabled: !!id,
  });
}

/** Records a service. Go clears the maintenance-due flag as part of it. */
export function useRecordMaintenance(id: string) {
  return assets.useAction<MaintenanceEvent>(id, 'maintenance');
}

const documents = createResourceHooks<Document, Document>('documents');
export const useDocument = documents.useOne;
export function useDocuments(query: StatusQuery) {
  return documents.useList(withStatus(query));
}
export const useCreateDocument = documents.useCreate;

// ---------------------------------------------------------------------------
// Администрирование
// ---------------------------------------------------------------------------

const roles = createResourceHooks<RoleDetail, RoleDetail>('admin/roles');
export function useRoles() {
  return roles.useList({});
}
// Custom roles are deliberately not offered in the UI.
//
// Five roles ship with the system so QOIM is not configuring RBAC on opening day
// (D9), and they are `is_system` — editable, not deletable. What an
// administrator actually needs is to change a role's PERMISSIONS and to assign
// roles to people, which the Администрирование screen does. Creating a sixth
// role is a decision with a permission matrix behind it, not a button.
//
// The Go endpoints exist and are tested; if the client asks for custom roles,
// the hooks are two lines. Leaving them defined and unreachable was the pattern
// that hid fourteen dead mutation paths, so they are not left lying around.

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
  /** Narrows the trail to a single record — what every detail view's activity
   *  panel asks for. Go already accepts it (`audit.sql:33`); nothing sent it. */
  resourceId?: string;
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
        filters: {
          resource: query.resource,
          resource_id: query.resourceId,
          actor_id: query.actorId,
        },
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
/**
 * The generic ladder mutation behind `WorkflowActions`.
 *
 * Every module's transition endpoint has the same shape — POST a target state to
 * a sub-resource of the record — so one hook serves all of them. Modules differ
 * only in the path and in which cache to drop, and both are arguments.
 */
export function useTransition(endpoint: string, invalidate: string | string[]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { to: string; reason?: string; comment?: string }) =>
      postJSON(endpoint, body),
    // Several keys, because a transition changes two things a user is usually
    // looking at: the record itself and the register it came from. Releasing a
    // batch that still showed as quarantined in the list behind the detail view
    // would be the same defect as not refetching at all.
    onSuccess: () =>
      Promise.all(
        (Array.isArray(invalidate) ? invalidate : [invalidate]).map((key) =>
          qc.invalidateQueries({ queryKey: [key] }),
        ),
      ),
  });
}

/**
 * Records a laboratory result against a batch.
 *
 * Append-only: there is no update and no delete, because a test result is
 * evidence. A wrong entry is corrected by recording another test, the same way
 * a wrong stock movement is corrected by a compensating entry.
 */
export function useRecordQualityTest(batchId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      test_type: string;
      result_value?: string;
      passed?: boolean;
      notes?: string;
    }) => postJSON(`/api/batches/${batchId}/tests`, body),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: ['batch-detail'] }),
        qc.invalidateQueries({ queryKey: ['quality/batches'] }),
      ]),
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

/**
 * Alt text is an accessibility obligation, so it is the one editable field on a
 * media record — and it is per-locale, because alt text is content rather than a
 * UI string (CLAUDE.md §6).
 */
export function useSetMediaAlt(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      alt_ru?: string;
      alt_tg?: string;
      alt_en?: string;
      version: number;
    }) => putJSON(`/api/cms/media/${id}/alt`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cms', 'media'] }),
  });
}

/**
 * The ladder's history for one piece of content.
 *
 * `content_workflow_events` is immutable evidence: who moved a page or a post
 * along the ladder, when, and what they said about it. Publishing is a claim the
 * company makes in public, so the record of who approved it is the point.
 */
export function useContentHistory(kind: 'pages' | 'news', id: string | undefined) {
  return useQuery<WorkflowEvent[]>({
    queryKey: ['cms', 'history', kind, id],
    queryFn: () => getJSON<WorkflowEvent[]>(`/api/cms/${kind}/${id}/history`),
    enabled: !!id,
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

/**
 * POSTs and returns the created record.
 *
 * Returned rather than discarded: a create form has to land on the record it
 * just made, and a helper that throws the response away forces every caller to
 * either re-fetch or navigate somewhere generic. Existing callers that ignore
 * the value are unaffected.
 */
async function postJSON<T = unknown>(path: string, payload: unknown): Promise<T> {
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
  // The created record, unwrapped from its {data} envelope. A 204 carries no
  // body — nothing in the create path returns one today, but a helper that
  // assumed otherwise would throw on the first endpoint that did.
  if (res.status === 204) return undefined as T;
  const created = await res.json().catch(() => null);
  return created?.data as T;
}
