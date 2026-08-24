import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import BatchCertificate from '@/app/print/batch/[id]/page';
import PurchaseOrderPrint from '@/app/print/purchase-order/[id]/page';
import ShipmentPrint from '@/app/print/shipment/[id]/page';
import { server, session, adminUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * R16 — printable documents.
 *
 * Implemented as print routes rather than server-side PDF: the gate is that
 * Cyrillic and Tajik render correctly, and the browser already loads the fonts
 * that were proved to do so. See PrintDocument's own comment.
 */

const ID = '018f9000-0000-7000-8000-000000000001';

vi.mock('next/navigation', () => ({
  useParams: () => ({ id: ID }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/print',
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

const status = (key: string, label: string, level = 'info') => ({ key, label, level });
const ok = (data: unknown) => HttpResponse.json({ data });

const RELEASED_BATCH = {
  batch: {
    id: ID,
    batch_no: 'B-2617',
    item_id: 'i1',
    produced_on: '2026-09-10',
    expires_on: '2027-09-10',
    qr_payload: null,
    qr_issued_at: null,
    status: status('released', 'Выпущена', 'ok'),
    version: 3,
    created_at: '2026-09-10T06:00:00Z',
  },
  sku: 'APJ-1000',
  item_name: 'Яблочный сок прямого отжима',
  tests: [
    {
      id: 't1',
      batch_id: ID,
      test_type: 'ph',
      result: status('passed', 'Пройдено', 'ok'),
      result_value: '3.6',
      tested_at: '2026-09-10T08:00:00Z',
      inspector_id: null,
      inspector: 'М. Назарова',
      notes: null,
    },
  ],
  history: [
    {
      id: 'e1',
      from_status: status('quarantine', 'Карантин', 'warn'),
      to_status: status('released', 'Выпущена', 'ok'),
      occurred_at: '2026-09-11T10:00:00Z',
      decided_by: 'u1',
      decider_name: 'С. Одинаев',
      reason: null,
    },
  ],
  stock: [],
  allowed_transitions: ['rejected'],
};

describe('Паспорт качества', () => {
  it('names the batch, its tests and the user who released it', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/batches/${ID}/detail`, () => ok(RELEASED_BATCH)));
    wrap(<BatchCertificate />);

    await screen.findByTestId('certificate-tests');
    // Once in the subtitle, once in the field block.
    expect(screen.getAllByText(/B-2617/).length).toBeGreaterThan(0);
    expect(screen.getByText('М. Назарова')).toBeInTheDocument();
    // A certificate that does not name a decision-maker is not evidence of a
    // decision.
    expect(screen.getByText('С. Одинаев')).toBeInTheDocument();
  });

  it('renders Cyrillic and Tajik characters, not entity escapes', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/batches/${ID}/detail`, () => ok(RELEASED_BATCH)));
    wrap(<BatchCertificate />);
    await screen.findByTestId('certificate-tests');
    // ҳ ҷ ӯ are why the font was chosen (CLAUDE.md §5).
    expect(screen.getAllByText(/Самари Кӯҳсор/).length).toBeGreaterThan(0);
  });

  it('refuses to certify a batch that has not been released', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/batches/${ID}/detail`, () =>
        ok({
          ...RELEASED_BATCH,
          batch: { ...RELEASED_BATCH.batch, status: status('quarantine', 'Карантин', 'warn') },
          history: [],
        }),
      ),
    );
    wrap(<BatchCertificate />);
    // Printing a certificate for quarantined stock would assert something untrue.
    expect(await screen.findByTestId('not-released')).toHaveTextContent('не выпущена');
  });

  it('says so plainly when no tests were performed', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/batches/${ID}/detail`, () => ok({ ...RELEASED_BATCH, tests: [] })),
    );
    wrap(<BatchCertificate />);
    expect(await screen.findByText('Испытания не проводились.')).toBeInTheDocument();
  });

  it('says the batch was not found rather than printing a blank certificate', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/batches/${ID}/detail`, () =>
        HttpResponse.json({ error: { code: 'not_found', message: '' } }, { status: 404 }),
      ),
    );
    wrap(<BatchCertificate />);
    expect(await screen.findByText('Партия не найдена.')).toBeInTheDocument();
  });
});

describe('Заказ поставщику', () => {
  it('lists the lines and totals them', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/purchase-orders/${ID}`, () =>
        ok({
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
              received_qty: '0.000',
              unit_price: '15.00',
              line_total: '15000.00',
            },
          ],
          allowed_transitions: [],
        }),
      ),
    );
    wrap(<PurchaseOrderPrint />);

    await screen.findByTestId('po-lines');
    expect(screen.getByText('Памир Агро')).toBeInTheDocument();
    expect(screen.getByTestId('po-total')).toHaveTextContent('15000.00 с.');
  });
});

describe('Товарно-транспортная накладная', () => {
  it('names the batch on every line, so the paper matches the chain', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/shipments/${ID}`, () =>
        ok({
          id: ID,
          trip_no: 'TR-77',
          route_from: 'Хорог',
          route_to: 'Душанбе',
          driver_id: 'd1',
          driver_name: 'Н. Шоев',
          vehicle_id: 'v1',
          vehicle_plate: '01 AA 123',
          transport_cost: '1200.00',
          status: status('in_transit', 'В пути'),
          version: 1,
          created_at: '2026-09-11T05:00:00Z',
          lines: [
            {
              id: 'sl1',
              item_id: 'i1',
              sku: 'APJ-1000',
              item_name: 'Яблочный сок',
              batch_id: 'b1',
              batch_no: 'B-2617',
              qty: '480.000',
            },
          ],
        }),
      ),
    );
    wrap(<ShipmentPrint />);

    await screen.findByTestId('shipment-lines');
    // The batch on the paper in the driver's cab is what a customer quotes back.
    expect(screen.getByText('B-2617')).toBeInTheDocument();
    expect(screen.getByText('Н. Шоев')).toBeInTheDocument();
  });
});
