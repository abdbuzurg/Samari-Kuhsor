'use client';

import type {
  Asset,
  BatchDetail,
  BatchListRow,
  Document,
  Employee,
  Inquiry,
  MaintenanceEvent,
  ManufacturingOrder,
  ManufacturingOrderRow,
  PurchaseOrder,
  PurchaseOrderRow,
  SalesOrder,
  SalesOrderRow,
  Shipment,
  StockBalanceRow,
  Supplier,
} from '@samari/types';

import { useQuery } from '@tanstack/react-query';

import { ApiError } from '@/lib/session';
import { createResourceHooks, type ListQuery } from '@/lib/resource';

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
