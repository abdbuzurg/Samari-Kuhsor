import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import ProductionDetail from '@/app/production/[id]/page';
import LogisticsDetail from '@/app/logistics/[id]/page';
import InquiryDetail from '@/app/inquiries/[id]/page';
import ProcurementDetail from '@/app/procurement/[id]/page';
import DocumentDetail from '@/app/documents/[id]/page';
import EquipmentDetail from '@/app/equipment/[id]/page';
import HRDetail from '@/app/hr/[id]/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * R04–R11 — the eight detail views built after the Качество reference slice.
 *
 * Every test names a role, an action and a browser. None asserts anything about
 * Go: the domain has 830 tests and none of them proved a human could reach it.
 */

const ID = '018f6000-0000-7000-8000-000000000001';

vi.mock('next/navigation', () => ({
  useParams: () => ({ id: ID }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}));

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

const noAudit = http.get('/api/audit', () =>
  HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
);
const ok = (data: unknown) => HttpResponse.json({ data });
const status = (key: string, label: string, level = 'info') => ({ key, label, level });

// --------------------------------------------------------------------------
// R04 · Производство
// --------------------------------------------------------------------------

const MO = {
  id: ID,
  mo_no: 'MO-104',
  item_id: 'i1',
  sku: 'APJ-1000',
  item_name: 'Яблочный сок прямого отжима',
  batch_id: 'b1',
  batch_no: 'B-2617',
  line: 'Линия 1',
  scheduled_for: '2026-09-10',
  planned_qty: '5000.000',
  good_qty: '4800.000',
  scrap_qty: '200.000',
  status: status('in_progress', 'В работе'),
  version: 3,
  created_at: '2026-09-10T05:00:00Z',
  progress: 96,
  yield_percent: '96.0',
  downtime_min: 25,
  entries: [
    {
      id: 'pe1',
      recorded_at: '2026-09-10T14:00:00Z',
      good_qty: '2400.000',
      scrap_qty: '100.000',
      downtime_min: 15,
      note: 'Смена 1',
      recorded_by: 'А. Раҳимов',
    },
  ],
};

describe('Производство — order detail', () => {
  it('shows planned, good, scrap and a yield that is computed, not stored', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/manufacturing-orders/${ID}`, () => ok(MO)), noAudit);
    wrap(<ProductionDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('4800.000')).toBeInTheDocument();
    expect(screen.getByText('96.0%')).toBeInTheDocument();
    expect(screen.getByTestId('related-row')).toHaveTextContent('Смена 1');
  });

  it('shows «уточняется» for yield before anything has run, not 0%', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/manufacturing-orders/${ID}`, () =>
        ok({ ...MO, yield_percent: null, good_qty: '0.000', entries: [] }),
      ),
      noAudit,
    );
    wrap(<ProductionDetail />);
    await screen.findByTestId('detail-view');
    // "0% yield" and "not started" read very differently on a shift report.
    expect(screen.queryByText('0%')).not.toBeInTheDocument();
    expect(screen.getByText('Записей по сменам ещё нет.')).toBeInTheDocument();
  });

  it('lets a production user record a shift entry', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/manufacturing-orders/${ID}`, () => ok(MO)),
      noAudit,
      http.post(`/api/manufacturing-orders/${ID}/entries`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<ProductionDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-entry-form'));
    await userEvent.type(screen.getByLabelText('Годных'), '2400');
    await userEvent.type(screen.getByLabelText('Брак'), '100');
    await userEvent.click(screen.getByTestId('save-entry'));

    await waitFor(() =>
      expect(sent).toMatchObject({ good_qty: '2400', scrap_qty: '100' }),
    );
  });

  it('completes an order and surfaces a second attempt being refused', async () => {
    let calls = 0;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/manufacturing-orders/${ID}`, () => ok(MO)),
      noAudit,
      http.post(`/api/manufacturing-orders/${ID}/complete`, () => {
        calls += 1;
        return HttpResponse.json(
          { error: { code: 'business_rule', message: 'Заказ уже завершён.' } },
          { status: 422 },
        );
      }),
    );
    wrap(<ProductionDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('complete-order'));
    expect(await screen.findByTestId('complete-error')).toHaveTextContent('Заказ уже завершён.');
    expect(calls).toBe(1);
  });

  it('hides completion from a user who may only read production', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['production:read'] }),
      http.get(`/api/manufacturing-orders/${ID}`, () => ok(MO)),
      noAudit,
    );
    wrap(<ProductionDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('complete-order')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toggle-entry-form')).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------
// R06 · Логистика
// --------------------------------------------------------------------------

const TRIP = {
  id: ID,
  trip_no: 'TR-77',
  route_from: 'Хорог',
  route_to: 'Душанбе',
  driver_id: 'd1',
  driver_name: 'Н. Шоев',
  vehicle_id: 'v1',
  vehicle_plate: '01 AA 123',
  transport_cost: '1200.00',
  status: status('loading', 'Погрузка', 'warn'),
  version: 1,
  created_at: '2026-09-11T05:00:00Z',
  lines: [],
};

const RELEASED_BATCHES = {
  data: [
    {
      id: 'b1',
      batch_no: 'B-2617',
      item_id: 'i1',
      sku: 'APJ-1000',
      item_name: 'Яблочный сок',
      produced_on: '2026-09-10',
      expires_on: '2027-09-10',
      test_count: 2,
      failed_count: 0,
      status: status('released', 'Выпущена', 'ok'),
      version: 3,
    },
  ],
  meta: { total: 1, page: 1, total_pages: 1 },
};

describe('Логистика — trip detail', () => {
  it('shows the route, driver and vehicle rather than three UUIDs', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/shipments/${ID}`, () => ok(TRIP)), noAudit);
    wrap(<LogisticsDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('Хорог → Душанбе')).toBeInTheDocument();
    expect(screen.getByText('Н. Шоев')).toBeInTheDocument();
    expect(screen.getByText('01 AA 123')).toBeInTheDocument();
  });

  it('offers only released batches for loading', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/shipments/${ID}`, () => ok(TRIP)),
      noAudit,
      http.get('/api/quality/batches', ({ request }) => {
        // A lorry leaving with quarantined product is the failure the whole
        // quality chain exists to prevent, so the picker asks for released only.
        expect(new URL(request.url).searchParams.get('status')).toBe('released');
        return HttpResponse.json(RELEASED_BATCHES);
      }),
    );
    wrap(<LogisticsDetail />);
    await screen.findByTestId('detail-view');
    await userEvent.click(screen.getByTestId('toggle-load-form'));
    expect(await screen.findByRole('option', { name: /B-2617/ })).toBeInTheDocument();
  });

  it('loads a batch, carrying its item across', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/shipments/${ID}`, () => ok(TRIP)),
      noAudit,
      http.get('/api/quality/batches', () => HttpResponse.json(RELEASED_BATCHES)),
      http.post(`/api/shipments/${ID}/lines`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<LogisticsDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-load-form'));
    await userEvent.selectOptions(await screen.findByLabelText('Партия'), 'b1');
    await userEvent.type(screen.getByLabelText('Количество'), '480');
    await userEvent.click(screen.getByTestId('save-load'));

    await waitFor(() =>
      expect(sent).toEqual({ item_id: 'i1', batch_id: 'b1', qty: '480' }),
    );
  });

  it('says so plainly when nothing has been released yet', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/shipments/${ID}`, () => ok(TRIP)),
      noAudit,
      http.get('/api/quality/batches', () =>
        HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
      ),
    );
    wrap(<LogisticsDetail />);
    await screen.findByTestId('detail-view');
    await userEvent.click(screen.getByTestId('toggle-load-form'));
    expect(await screen.findByTestId('no-released')).toHaveTextContent('Выпущенных партий нет');
  });

  it('shows a single «уточняется» for a trip with neither end of its route set', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/shipments/${ID}`, () => ok({ ...TRIP, route_from: null, route_to: null })),
      noAudit,
    );
    wrap(<LogisticsDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getAllByText('уточняется').length).toBeGreaterThan(0);
  });
});

// --------------------------------------------------------------------------
// R07 · Обращения
// --------------------------------------------------------------------------

const COMPLAINT = {
  id: ID,
  reference_no: 'CP-0007',
  type: status('complaint', 'Жалоба', 'danger'),
  name: 'А. Каримов',
  company: null,
  contact: '+992 900 111 222',
  message: 'Посторонний привкус',
  batch_id: 'b1',
  batch_no: 'B-2617',
  status: status('new', 'Новое', 'warn'),
  submitted_at: '2026-09-12T09:00:00Z',
  version: 1,
};

describe('Интеграция с сайтом — enquiry detail', () => {
  it('leads with the reference number the visitor holds', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/inquiries/${ID}`, () => ok(COMPLAINT)), noAudit);
    wrap(<InquiryDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getAllByText('CP-0007').length).toBeGreaterThan(0);
  });

  it('links a complaint to its batch, so traceability has an entry point', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/inquiries/${ID}`, () => ok(COMPLAINT)), noAudit);
    wrap(<InquiryDetail />);
    await screen.findByTestId('detail-view');
    const link = screen.getByRole('link', { name: 'B-2617' });
    expect(link).toHaveAttribute('href', '/quality/b1');
  });

  it('shows no traceability band for an enquiry that is not a complaint', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/inquiries/${ID}`, () =>
        ok({ ...COMPLAINT, type: status('wholesale', 'Оптовый запрос'), batch_id: null, batch_no: null }),
      ),
      noAudit,
    );
    wrap(<InquiryDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByText('Прослеживаемость')).not.toBeInTheDocument();
  });

  it('lets a sales user convert a new enquiry into a lead', async () => {
    let called = false;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/inquiries/${ID}`, () => ok(COMPLAINT)),
      noAudit,
      http.post(`/api/inquiries/${ID}/convert`, () => {
        called = true;
        return ok({ id: 'l1', customer_id: 'c1' });
      }),
    );
    wrap(<InquiryDetail />);
    await screen.findByTestId('detail-view');
    await userEvent.click(screen.getByTestId('convert-inquiry'));
    await waitFor(() => expect(called).toBe(true));
  });

  it('offers no conversion once the enquiry has already been processed', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/inquiries/${ID}`, () =>
        ok({ ...COMPLAINT, status: status('lead_created', 'Создан лид', 'ok') }),
      ),
      noAudit,
    );
    wrap(<InquiryDetail />);
    await screen.findByTestId('detail-view');
    // Converting twice would put two leads behind one enquiry.
    expect(screen.queryByTestId('convert-inquiry')).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------
// R08 · Закупки
// --------------------------------------------------------------------------

const PO = {
  id: ID,
  po_no: 'PO-31',
  supplier_id: 's1',
  supplier_name: 'Памир Агро',
  expected_at: '2026-09-15',
  total: '15000.00',
  status: status('confirmed', 'Подтверждён', 'ok'),
  version: 2,
  created_at: '2026-09-01T05:00:00Z',
  lines: [
    {
      id: 'pl1',
      item_id: 'i2',
      sku: 'RAW-APPLE',
      item_name: 'Яблоки',
      qty: '1000.000',
      received_qty: '400.000',
      unit_price: '15.00',
      line_total: '15000.00',
    },
  ],
  allowed_transitions: ['receiving', 'cancelled'],
};

describe('Закупки — purchase order detail', () => {
  it('shows the lines and flags one that is only partly received', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/purchase-orders/${ID}`, () => ok(PO)), noAudit);
    wrap(<ProcurementDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('Яблоки')).toBeInTheDocument();
    expect(screen.getByText('400.000').tagName).toBe('STRONG');
  });

  it('renders only the transitions the server allowed', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/purchase-orders/${ID}`, () => ok(PO)), noAudit);
    wrap(<ProcurementDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByRole('button', { name: 'К приёмке' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Подтвердить' })).not.toBeInTheDocument();
  });

  it('lets a procurement user receive goods against the order', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/purchase-orders/${ID}`, () => ok(PO)),
      noAudit,
      http.get('/api/locations', () => ok([{ id: 'loc1', code: 'RM-01', name: 'Сырьё', zone: 'raw' }])),
      http.post(`/api/purchase-orders/${ID}/receipts`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<ProcurementDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-receipt'));
    await userEvent.selectOptions(await screen.findByLabelText('Локация'), 'loc1');
    await userEvent.type(screen.getByLabelText('Принято по RAW-APPLE'), '600');
    await userEvent.click(screen.getByTestId('save-receipt'));

    await waitFor(() =>
      expect(sent).toEqual({
        location_id: 'loc1',
        lines: [{ po_line_id: 'pl1', qty: '600' }],
      }),
    );
  });

  it('refuses to submit a receipt with no quantities', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/purchase-orders/${ID}`, () => ok(PO)),
      noAudit,
      http.get('/api/locations', () => ok([{ id: 'loc1', code: 'RM-01', name: 'Сырьё', zone: 'raw' }])),
    );
    wrap(<ProcurementDetail />);
    await screen.findByTestId('detail-view');
    await userEvent.click(screen.getByTestId('toggle-receipt'));
    await userEvent.selectOptions(await screen.findByLabelText('Локация'), 'loc1');
    await userEvent.click(screen.getByTestId('save-receipt'));
    expect(await screen.findByTestId('receipt-error')).toHaveTextContent('хотя бы по одной позиции');
  });

  it('hides receiving on an order that is still a draft', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/purchase-orders/${ID}`, () =>
        ok({ ...PO, status: status('draft', 'Черновик', 'neutral'), allowed_transitions: ['approval'] }),
      ),
      noAudit,
    );
    wrap(<ProcurementDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('toggle-receipt')).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------
// R09 · Документы
// --------------------------------------------------------------------------

describe('Документы — document detail', () => {
  const DOC = {
    id: ID,
    doc_no: 'D-014',
    title: 'ISO 22000',
    doc_type: 'certificate',
    owner_id: 'u1',
    owner_name: 'С. Одинаев',
    valid_until: '2027-01-01',
    status: status('approval', 'На согласовании', 'warn'),
    version: 2,
    allowed_transitions: ['active', 'draft'],
  };

  it('offers activation only when the server allowed it', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/documents/${ID}`, () => ok(DOC)), noAudit);
    wrap(<DocumentDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByRole('button', { name: 'Ввести в действие' })).toBeInTheDocument();
  });

  it('offers no activation to a user without documents:approve', async () => {
    server.use(
      session.loaded(adminUser),
      // The server computed the list; without approve it comes back shorter.
      http.get(`/api/documents/${ID}`, () => ok({ ...DOC, allowed_transitions: ['draft'] })),
      noAudit,
    );
    wrap(<DocumentDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByRole('button', { name: 'Ввести в действие' })).not.toBeInTheDocument();
  });

  it('never offers «expired» as a decision', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/documents/${ID}`, () =>
        ok({ ...DOC, status: status('expired', 'Просрочен', 'danger'), allowed_transitions: [] }),
      ),
      noAudit,
    );
    wrap(<DocumentDetail />);
    await screen.findByTestId('detail-view');
    // Expiry is a condition of a date passing, not something anyone decides.
    expect(screen.getByTestId('no-transitions')).toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------
// R10 · Оборудование
// --------------------------------------------------------------------------

describe('Оборудование — asset detail', () => {
  const ASSET = {
    id: ID,
    asset_no: 'EQ-047',
    name: 'Линия розлива',
    asset_type: 'filling',
    line: 'Линия 1',
    commissioned_on: '2026-08-01',
    warranty_until: '2028-08-01',
    next_due_on: '2026-12-01',
    last_service_at: '2026-09-01T08:00:00Z',
    status: status('running', 'В работе', 'ok'),
    version: 1,
  };

  it('answers «when next» on the record, not only in the register', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/assets/${ID}`, () => ok(ASSET)),
      http.get(`/api/assets/${ID}/maintenance`, () => ok([])),
      noAudit,
    );
    wrap(<EquipmentDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('2026-12-01')).toBeInTheDocument();
  });

  it('lets an equipment user record a service', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/assets/${ID}`, () => ok(ASSET)),
      http.get(`/api/assets/${ID}/maintenance`, () => ok([])),
      noAudit,
      http.post(`/api/assets/${ID}/maintenance`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<EquipmentDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-maintenance-form'));
    await userEvent.selectOptions(screen.getByLabelText('Тип'), 'breakdown');
    await userEvent.click(screen.getByTestId('save-maintenance'));

    await waitFor(() => expect(sent).toMatchObject({ event_type: 'breakdown' }));
  });

  it('hides the service form from a viewer who may only read', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['equipment:read'] }),
      http.get(`/api/assets/${ID}`, () => ok(ASSET)),
      http.get(`/api/assets/${ID}/maintenance`, () => ok([])),
      noAudit,
    );
    wrap(<EquipmentDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('toggle-maintenance-form')).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------
// R11 · Персонал
// --------------------------------------------------------------------------

describe('Персонал — employee file', () => {
  const EMPLOYEE = {
    id: ID,
    full_name: 'М. Назарова',
    position_id: 'p1',
    position_title: 'Лаборант',
    department: 'Качество',
    shift: 'day',
    hired_on: '2026-08-15',
    contract_until: '2027-08-15',
    status: status('active', 'Работает', 'ok'),
    version: 4,
  };

  it('renders the shift in Russian rather than the stored key', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/employees/${ID}`, () => ok(EMPLOYEE)), noAudit);
    wrap(<HRDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('Дневная')).toBeInTheDocument();
    expect(screen.queryByText('day')).not.toBeInTheDocument();
  });

  it('sends the current version so a stale edit is refused rather than silent', async () => {
    let sent: { version?: number } | null = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/employees/${ID}`, () => ok(EMPLOYEE)),
      noAudit,
      http.patch(`/api/employees/${ID}`, async ({ request }) => {
        sent = (await request.json()) as { version?: number };
        return ok({ ...EMPLOYEE, version: 5 });
      }),
    );
    wrap(<HRDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-edit'));
    await userEvent.click(screen.getByTestId('save-employee'));

    await waitFor(() => expect(sent?.version).toBe(4));
  });

  it('hides editing from a user who may only read personnel', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['hr:read'] }),
      http.get(`/api/employees/${ID}`, () => ok(EMPLOYEE)),
      noAudit,
    );
    wrap(<HRDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('toggle-edit')).not.toBeInTheDocument();
  });
});
