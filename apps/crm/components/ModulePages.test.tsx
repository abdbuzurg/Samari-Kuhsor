import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import InventoryPage from '@/app/inventory/page';
import ProductionPage from '@/app/production/page';
import ProcurementPage from '@/app/procurement/page';
import SalesPage from '@/app/crm/page';
import LogisticsPage from '@/app/logistics/page';
import InquiriesPage from '@/app/inquiries/page';
import QualityPage from '@/app/quality/page';
import { server, session, adminUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type {
  BatchListRow,
  Inquiry,
  ManufacturingOrderRow,
  PurchaseOrderRow,
  SalesOrderRow,
  Shipment,
  StockBalanceRow,
} from '@samari/types';

/**
 * CLAUDE.md §7 — every React data component is tested in four states: loading,
 * empty, error and populated.
 *
 * Six modules share one file because after the T15 extraction they share one
 * implementation: each page is a column config over the same ListView, so the
 * interesting assertions are about the config, not about six copies of the
 * loading spinner. What is module-specific gets its own test below.
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

/** A collection response in the contract's envelope (docs/03-API-CONTRACT.md §4). */
function collection(path: string, rows: unknown[], total = rows.length) {
  return http.get(path, () =>
    HttpResponse.json({
      data: rows,
      meta: { page: 1, per_page: 25, total, total_pages: Math.max(1, Math.ceil(total / 25)) },
    }),
  );
}

function loadingForever(path: string) {
  return http.get(path, async () => {
    await delay('infinite');
    return HttpResponse.json({ data: [] });
  });
}

function failing(path: string) {
  return http.get(path, () =>
    HttpResponse.json(
      { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
      { status: 500 },
    ),
  );
}

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

const SUGAR: StockBalanceRow = {
  item_id: '018f3c9e-0000-7000-8000-000000000001',
  sku: 'SUG-25',
  item_name: 'Сахар-песок',
  base_uom: 'kg',
  batch_id: null,
  batch_no: null,
  batch_status: null,
  expires_on: null,
  location_id: '018f3c9e-0000-7000-8000-0000000000a1',
  location_code: 'RAW-1',
  location_zone: 'raw',
  // Fractional on purpose: numeric(14,3) exists so raw materials can be issued
  // in partial units, and this asserts the string survives to the screen.
  on_hand: '12.500',
  min_qty: '100.000',
  status: { key: 'below_minimum', label: 'Ниже минимума', level: 'danger' },
  last_movement_at: '2026-08-17T08:00:00Z',
};

const JUICE: StockBalanceRow = {
  ...SUGAR,
  item_id: '018f3c9e-0000-7000-8000-000000000002',
  sku: 'APJ-1000',
  item_name: 'Яблочный сок 1 л',
  base_uom: 'bottle',
  batch_id: '018f3c9e-0000-7000-8000-0000000000b1',
  batch_no: 'B-2617',
  batch_status: { key: 'quarantine', label: 'Карантин', level: 'danger' },
  location_id: '018f3c9e-0000-7000-8000-0000000000a2',
  location_code: 'QUAR-1',
  location_zone: 'quarantine',
  on_hand: '4800.000',
  min_qty: null,
  status: { key: 'tracked', label: 'Без минимума', level: 'neutral' },
};

const MO: ManufacturingOrderRow = {
  id: '018f3c9e-0000-7000-8000-0000000000c1',
  mo_no: 'MO-000012',
  item_id: JUICE.item_id,
  sku: 'APJ-1000',
  item_name: 'Яблочный сок 1 л',
  batch_id: JUICE.batch_id,
  batch_no: 'B-2617',
  line: 'Линия 1',
  scheduled_for: '2026-08-18',
  planned_qty: '5000.000',
  good_qty: '4800.000',
  scrap_qty: '120.000',
  status: { key: 'in_progress', label: 'В работе', level: 'info' },
  version: 1,
  created_at: '2026-08-17T06:00:00Z',
};

const PO: PurchaseOrderRow = {
  id: '018f3c9e-0000-7000-8000-0000000000d1',
  po_no: 'PO-000004',
  supplier_id: '018f3c9e-0000-7000-8000-0000000000e1',
  supplier_name: 'Ориён Агро',
  expected_at: '2026-08-25',
  total: '48200.00',
  status: { key: 'approval', label: 'На согласовании', level: 'warn' },
  version: 1,
  created_at: '2026-08-17T06:00:00Z',
};

const SO: SalesOrderRow = {
  id: '018f3c9e-0000-7000-8000-0000000000f1',
  so_no: 'SO-000021',
  customer_id: '018f3c9e-0000-7000-8000-000000000101',
  customer_name: 'Маркет Хорог',
  ordered_on: '2026-08-16',
  total: '12400.00',
  status: { key: 'draft', label: 'Черновик', level: 'neutral' },
  version: 1,
  created_at: '2026-08-16T06:00:00Z',
};

const TRIP: Shipment = {
  id: '018f3c9e-0000-7000-8000-000000000111',
  trip_no: 'TR-000007',
  route_from: 'Хорог',
  route_to: 'Душанбе',
  driver_id: null,
  driver_name: 'С. Назаров',
  vehicle_id: null,
  vehicle_plate: '01 AA 234',
  transport_cost: '1850.00',
  status: { key: 'in_transit', label: 'В пути', level: 'info' },
  version: 1,
  created_at: '2026-08-17T04:00:00Z',
  lines: [],
};

const BATCH: BatchListRow = {
  id: '018f3c9e-0000-7000-8000-0000000000b1',
  batch_no: 'B-2617',
  item_id: JUICE.item_id,
  sku: 'APJ-1000',
  item_name: 'Яблочный сок 1 л',
  produced_on: '2026-08-17',
  expires_on: '2027-08-17',
  test_count: 3,
  failed_count: 1,
  status: { key: 'quarantine', label: 'Карантин', level: 'danger' },
  version: 1,
};

const COMPLAINT: Inquiry = {
  id: '018f3c9e-0000-7000-8000-000000000121',
  reference_no: 'CP-000001',
  type: { key: 'complaint', label: 'Жалоба', level: 'danger' },
  name: 'Н. Сафарова',
  company: null,
  contact: '+992 000 00 00',
  message: 'Осадок в бутылке',
  batch_id: JUICE.batch_id,
  batch_no: 'B-2617',
  status: { key: 'new', label: 'Новое', level: 'warn' },
  submitted_at: '2026-08-17T09:00:00Z',
  version: 1,
};

// Every module, with the endpoint it reads and a row to render. The four-state
// tests below run over this table so a new module cannot be added without them.
// `search` is the module toolbar's placeholder. It is needed because the app
// shell also renders a global search box, so `getByRole('searchbox')` finds two.
const MODULES = [
  {
    name: 'Склад', path: '/api/stock', Page: InventoryPage, row: SUGAR,
    cell: 'Сахар-песок', search: 'Поиск по артикулу, наименованию и партии',
  },
  {
    name: 'Производство', path: '/api/manufacturing-orders', Page: ProductionPage, row: MO,
    cell: 'MO-000012', search: 'Поиск по номеру заказа и артикулу',
  },
  {
    name: 'Закупки', path: '/api/purchase-orders', Page: ProcurementPage, row: PO,
    cell: 'Ориён Агро', search: 'Поиск по номеру заказа и поставщику',
  },
  {
    name: 'Продажи', path: '/api/sales-orders', Page: SalesPage, row: SO,
    cell: 'Маркет Хорог', search: 'Поиск по номеру заказа и клиенту',
  },
  {
    name: 'Логистика', path: '/api/shipments', Page: LogisticsPage, row: TRIP,
    cell: 'TR-000007', search: 'Поиск по номеру рейса',
  },
  {
    name: 'Обращения', path: '/api/inquiries', Page: InquiriesPage, row: COMPLAINT,
    cell: 'CP-000001', search: 'Поиск по номеру, имени и компании',
  },
  {
    name: 'Качество', path: '/api/quality/batches', Page: QualityPage, row: BATCH,
    cell: 'B-2617', search: 'Поиск по номеру партии и продукции',
  },
] as const;

describe.each(MODULES)('$name', ({ path, Page, row, cell, search }) => {
  it('renders its rows once loaded', async () => {
    server.use(session.loaded(adminUser), collection(path, [row]));
    wrap(<Page />);
    expect(await screen.findByText(cell)).toBeInTheDocument();
  });

  it('shows a loading state while the request is in flight', async () => {
    server.use(session.loaded(adminUser), loadingForever(path));
    wrap(<Page />);
    await waitFor(() => {
      expect(screen.getByTestId('list-loading')).toBeInTheDocument();
    });
    expect(screen.queryByText(cell)).not.toBeInTheDocument();
  });

  it('shows the empty state, not an error, when the collection is empty', async () => {
    server.use(session.loaded(adminUser), collection(path, []));
    wrap(<Page />);
    expect(await screen.findByTestId('list-empty')).toBeInTheDocument();
  });

  it('shows an error state when the request fails', async () => {
    server.use(session.loaded(adminUser), failing(path));
    wrap(<Page />);
    expect(await screen.findByTestId('list-error')).toBeInTheDocument();
    // And never a false empty state — "нет данных" and "не удалось загрузить"
    // lead the user to completely different actions.
    expect(screen.queryByTestId('list-empty')).not.toBeInTheDocument();
  });

  it('shows the no-match state when a search excludes everything', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(path, ({ request }) => {
        const q = new URL(request.url).searchParams.get('q');
        return HttpResponse.json({
          data: q ? [] : [row],
          meta: { page: 1, per_page: 25, total: q ? 0 : 1, total_pages: 1 },
        });
      }),
    );
    wrap(<Page />);
    expect(await screen.findByText(cell)).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText(search), 'ничего');
    await waitFor(() => {
      expect(screen.getByTestId('list-no-match')).toBeInTheDocument();
    });
    // Not the empty state: the collection is not empty, the filter is narrow.
    expect(screen.queryByTestId('list-empty')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// What is actually module-specific
// ---------------------------------------------------------------------------

describe('Склад', () => {
  // The defect this guards: a fractional quantity rendered through Number()
  // silently loses precision, and numeric(14,3) exists precisely so raw
  // materials can be issued in partial units (CLAUDE.md §4.7).
  it('renders quantities as the exact strings the API sent', async () => {
    server.use(session.loaded(adminUser), collection('/api/stock', [SUGAR, JUICE]));
    wrap(<InventoryPage />);
    expect(await screen.findByText('12.500 kg')).toBeInTheDocument();
    expect(screen.getByText('4800.000 bottle')).toBeInTheDocument();
  });

  it('marks a position below its minimum as danger, and one with no minimum as neutral', async () => {
    server.use(session.loaded(adminUser), collection('/api/stock', [SUGAR, JUICE]));
    wrap(<InventoryPage />);
    await screen.findByText('Сахар-песок');

    const tags = screen.getAllByTestId('status-tag');
    const below = tags.find((el) => el.dataset.status === 'below_minimum');
    expect(below).toHaveAttribute('data-level', 'danger');

    const tracked = tags.find((el) => el.dataset.status === 'tracked');
    expect(tracked).toHaveAttribute('data-level', 'neutral');
  });

  it('shows «уточняется» rather than an empty cell for an absent minimum', async () => {
    server.use(session.loaded(adminUser), collection('/api/stock', [JUICE]));
    wrap(<InventoryPage />);
    await screen.findByText('Яблочный сок 1 л');
    // An empty cell reads as "no minimum required"; «уточняется» is the truth.
    expect(screen.getAllByText('уточняется').length).toBeGreaterThan(0);
  });

  it('counts positions below minimum in its KPI strip', async () => {
    server.use(session.loaded(adminUser), collection('/api/stock', [SUGAR, JUICE]));
    wrap(<InventoryPage />);
    await screen.findByText('Сахар-песок');
    // Scoped to the KPI strip: «Ниже минимума» is also a status tag label, and
    // an unscoped query would find the row's tag instead of the count.
    const strip = screen.getByTestId('kpi-strip');
    const kpi = within(strip).getByText('Ниже минимума').closest('[data-testid="kpi"]');
    expect(kpi).toHaveTextContent('1');
  });
});

describe('Производство', () => {
  it('shows planned, good and scrap as separate columns', async () => {
    server.use(session.loaded(adminUser), collection('/api/manufacturing-orders', [MO]));
    wrap(<ProductionPage />);
    await screen.findByText('MO-000012');
    expect(screen.getByText('5000.000')).toBeInTheDocument();
    expect(screen.getByText('4800.000')).toBeInTheDocument();
    // Scrap is shown, not folded into a yield percentage: the shift supervisor
    // needs the number they can act on.
    expect(screen.getByText('120.000')).toBeInTheDocument();
  });
});

describe('Закупки', () => {
  it('flags orders waiting for approval, because they are blocking someone', async () => {
    server.use(session.loaded(adminUser), collection('/api/purchase-orders', [PO]));
    wrap(<ProcurementPage />);
    await screen.findByText('Ориён Агро');

    const tag = screen.getAllByTestId('status-tag').find((el) => el.dataset.status === 'approval');
    expect(tag).toHaveAttribute('data-level', 'warn');
    expect(screen.getByText('требует решения')).toBeInTheDocument();
  });

  it('renders money as the exact string, with no float round trip', async () => {
    server.use(session.loaded(adminUser), collection('/api/purchase-orders', [PO]));
    wrap(<ProcurementPage />);
    expect(await screen.findByText('48200.00 с.')).toBeInTheDocument();
  });
});

describe('Логистика', () => {
  it('shows the driver and vehicle, not two UUIDs', async () => {
    server.use(session.loaded(adminUser), collection('/api/shipments', [TRIP]));
    wrap(<LogisticsPage />);
    await screen.findByText('TR-000007');
    expect(screen.getByText('С. Назаров')).toBeInTheDocument();
    expect(screen.getByText('01 AA 234')).toBeInTheDocument();
    expect(screen.getByText('Хорог → Душанбе')).toBeInTheDocument();
  });

  it('shows a single «уточняется» for a trip with neither end of its route set', async () => {
    const unplanned: Shipment = { ...TRIP, route_from: null, route_to: null };
    server.use(session.loaded(adminUser), collection('/api/shipments', [unplanned]));
    wrap(<LogisticsPage />);
    await screen.findByText('TR-000007');
    // Not «уточняется → уточняется», which is noise rather than information.
    expect(screen.queryByText('уточняется → уточняется')).not.toBeInTheDocument();
  });
});

describe('Качество', () => {
  it('shows a quarantined batch as danger and flags it as awaiting a decision', async () => {
    server.use(session.loaded(adminUser), collection('/api/quality/batches', [BATCH]));
    wrap(<QualityPage />);
    await screen.findByText('B-2617');

    const tag = screen.getAllByTestId('status-tag').find((el) => el.dataset.status === 'quarantine');
    expect(tag).toHaveAttribute('data-level', 'danger');
    expect(screen.getByText('ожидают решения')).toBeInTheDocument();
  });

  it('calls out a failed test in the list, not only in the detail view', async () => {
    server.use(session.loaded(adminUser), collection('/api/quality/batches', [BATCH]));
    wrap(<QualityPage />);
    // The point of the list is spotting the batch that needs attention without
    // opening twenty pages.
    expect(await screen.findByText('3 (1 — не соотв.)')).toBeInTheDocument();
  });

  it('shows a bare count when nothing failed', async () => {
    const clean: BatchListRow = {
      ...BATCH,
      batch_no: 'B-2618',
      failed_count: 0,
      status: { key: 'released', label: 'Выпущено', level: 'ok' },
    };
    server.use(session.loaded(adminUser), collection('/api/quality/batches', [clean]));
    wrap(<QualityPage />);
    await screen.findByText('B-2618');
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.queryByText(/не соотв/)).not.toBeInTheDocument();
  });
});

describe('Интеграция с сайтом', () => {
  it('renders a complaint as danger and calls it out in the KPI strip', async () => {
    server.use(session.loaded(adminUser), collection('/api/inquiries', [COMPLAINT]));
    wrap(<InquiriesPage />);
    await screen.findByText('CP-000001');

    const tag = screen.getAllByTestId('status-tag').find((el) => el.dataset.status === 'complaint');
    expect(tag).toHaveAttribute('data-level', 'danger');
    expect(screen.getByText('требует расследования')).toBeInTheDocument();
  });

  it('shows the batch a complaint names, so traceability has an entry point', async () => {
    server.use(session.loaded(adminUser), collection('/api/inquiries', [COMPLAINT]));
    wrap(<InquiriesPage />);
    expect(await screen.findByText('B-2617')).toBeInTheDocument();
  });

  it('does not raise the complaint banner when there are none', async () => {
    const enquiry: Inquiry = {
      ...COMPLAINT,
      reference_no: 'CF-000009',
      type: { key: 'contact', label: 'Обращение', level: 'neutral' },
      batch_id: null,
      batch_no: null,
    };
    server.use(session.loaded(adminUser), collection('/api/inquiries', [enquiry]));
    wrap(<InquiriesPage />);
    await screen.findByText('CF-000009');
    expect(screen.queryByText('требует расследования')).not.toBeInTheDocument();
  });
});
