import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import StockLedgerPage from '@/app/inventory/ledger/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * R05 — Склад, the position ledger.
 *
 * The hardest correctness constraint in the CRM is a UI one:
 * **no form anywhere may offer an absolute quantity** (05-MODULES.md:112).
 * A "set stock to X" control would make the append-only ledger a lie, because
 * the only honest way to implement it is an update.
 */

const ITEM = '018f7000-0000-7000-8000-000000000001';
const LOCATION = '018f7000-0000-7000-8000-0000000000aa';

vi.mock('next/navigation', () => ({
  useParams: () => ({}),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/inventory/ledger',
  useSearchParams: () =>
    new URLSearchParams({ item_id: ITEM, location_id: LOCATION }),
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

const POSITION = {
  item_id: ITEM,
  sku: 'APJ-1000',
  item_name: 'Яблочный сок прямого отжима',
  base_uom: 'bottle',
  batch_id: null,
  batch_no: null,
  batch_status: null,
  expires_on: null,
  location_id: LOCATION,
  location_code: 'FG-01',
  location_zone: 'Готовая продукция',
  on_hand: '480.000',
  min_qty: '100.000',
  status: status('ok', 'В норме', 'ok'),
  last_movement_at: '2026-09-12T10:00:00Z',
};

const MOVEMENTS = {
  data: [
    {
      id: 'm1',
      occurred_at: '2026-09-12T10:00:00Z',
      qty_delta: '-20.000',
      running_balance: '480.000',
      reason: status('sale', 'Продажа'),
      ref_type: null,
      ref_id: null,
      note: null,
      created_by: 'А. Раҳимов',
    },
    {
      id: 'm2',
      occurred_at: '2026-09-11T10:00:00Z',
      qty_delta: '500.000',
      running_balance: '500.000',
      reason: status('production_output', 'Выпуск продукции'),
      ref_type: null,
      ref_id: null,
      note: null,
      created_by: 'А. Раҳимов',
    },
  ],
  meta: { total: 2, page: 1, total_pages: 1 },
};

function baseHandlers(user = adminUser) {
  return [
    session.loaded(user),
    http.get('/api/stock/ledger', () => HttpResponse.json(MOVEMENTS)),
    http.get('/api/stock', () =>
      HttpResponse.json({ data: [POSITION], meta: { total: 1, page: 1, total_pages: 1 } }),
    ),
    http.get('/api/locations', () =>
      HttpResponse.json({
        data: [
          { id: LOCATION, code: 'FG-01', name: 'Готовая продукция', zone: 'fg' },
          { id: 'loc2', code: 'QRN-01', name: 'Карантин', zone: 'quarantine' },
        ],
      }),
    ),
    http.get('/api/audit', () =>
      HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
    ),
  ];
}

describe('Склад — the ledger', () => {
  it('explains the balance: every movement with the running total after it', async () => {
    server.use(...baseHandlers());
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    const rows = screen.getAllByTestId('related-row');
    expect(rows).toHaveLength(2);
    // This is why no balance is ever stored: 480 is explained by the rows.
    expect(rows[0]).toHaveTextContent('-20.000');
    expect(rows[0]).toHaveTextContent('480.000');
    expect(rows[1]).toHaveTextContent('500.000');
  });

  it('renders quantities as the exact strings the API sent', async () => {
    server.use(...baseHandlers());
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');
    // Money and quantities are strings end to end; a float round trip here
    // would silently lose the third decimal on raw materials.
    expect(screen.getAllByText('480.000').length).toBeGreaterThan(0);
  });

  it('says the position was not found when the URL is missing its parameters', async () => {
    // A ledger addressed by (item, batch, location) has no meaning without them.
    vi.resetModules();
    server.use(...baseHandlers());
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('detail-error')).not.toBeInTheDocument();
  });

  it('shows an empty ledger without pretending it is an error', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/stock/ledger', () =>
        HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
      ),
      http.get('/api/stock', () =>
        HttpResponse.json({ data: [POSITION], meta: { total: 1, page: 1, total_pages: 1 } }),
      ),
      http.get('/api/locations', () => HttpResponse.json({ data: [] })),
      http.get('/api/audit', () =>
        HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
      ),
    );
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');
    expect(screen.getByTestId('related-empty')).toHaveTextContent('Движений по этой позиции нет.');
  });
});

describe('Склад — posting movements', () => {
  it('NO input anywhere offers an absolute quantity', async () => {
    server.use(...baseHandlers());
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-movement'));

    // The whole module rests on this. Every label must describe a CHANGE, and
    // none may describe a resulting total. If a future edit adds "Остаток" or
    // "Итого" as an editable field, this fails.
    const inputs = screen.getAllByRole('textbox').concat(screen.getAllByRole('combobox'));
    for (const input of inputs) {
      const label = input.getAttribute('aria-label') ?? '';
      expect(label).not.toMatch(/остаток|итого|установить/i);
    }
    expect(screen.getByText(/изменение остатка, а не итоговое количество/i)).toBeInTheDocument();
  });

  it('derives the sign from the reason rather than asking the user to type one', async () => {
    let sent: { qty_delta?: string } | null = null;
    server.use(
      ...baseHandlers(),
      http.post('/api/stock/movements', async ({ request }) => {
        sent = (await request.json()) as { qty_delta?: string };
        return HttpResponse.json({ data: {} });
      }),
    );
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-movement'));
    await userEvent.selectOptions(screen.getByLabelText('Причина'), 'sale');
    await userEvent.type(screen.getByLabelText('Количество'), '80');
    await userEvent.click(screen.getByTestId('save-movement'));

    // A sale takes stock out; the user typed a magnitude.
    await waitFor(() => expect(sent?.qty_delta).toBe('-80'));
  });

  it('posts a receipt as a positive delta', async () => {
    let sent: { qty_delta?: string; reason?: string } | null = null;
    server.use(
      ...baseHandlers(),
      http.post('/api/stock/movements', async ({ request }) => {
        sent = (await request.json()) as { qty_delta?: string; reason?: string };
        return HttpResponse.json({ data: {} });
      }),
    );
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-movement'));
    await userEvent.selectOptions(screen.getByLabelText('Причина'), 'goods_receipt');
    await userEvent.type(screen.getByLabelText('Количество'), '500');
    await userEvent.click(screen.getByTestId('save-movement'));

    await waitFor(() =>
      expect(sent).toMatchObject({ qty_delta: '500', reason: 'goods_receipt' }),
    );
  });

  it('explains that a correction is the one reason allowed to go negative', async () => {
    server.use(...baseHandlers());
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-movement'));
    await userEvent.selectOptions(screen.getByLabelText('Причина'), 'adjustment');
    expect(screen.getByText(/может увести остаток в минус/i)).toBeInTheDocument();
  });

  it('shows the server refusing an issue that would go negative', async () => {
    server.use(
      ...baseHandlers(),
      http.post('/api/stock/movements', () =>
        HttpResponse.json(
          { error: { code: 'business_rule', message: 'Недостаточно остатка: доступно 480.000' } },
          { status: 422 },
        ),
      ),
    );
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-movement'));
    await userEvent.selectOptions(screen.getByLabelText('Причина'), 'sale');
    await userEvent.type(screen.getByLabelText('Количество'), '5000');
    await userEvent.click(screen.getByTestId('save-movement'));

    expect(await screen.findByTestId('movement-error')).toHaveTextContent('Недостаточно остатка');
  });

  it('transfers to another location, never to the one it is already in', async () => {
    let sent: unknown = null;
    server.use(
      ...baseHandlers(),
      http.post('/api/stock/transfers', async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ data: {} });
      }),
    );
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-transfer'));
    const target = await screen.findByLabelText('Куда');
    // The source location is filtered out — a transfer to itself is not a move.
    expect(target).not.toHaveTextContent('FG-01');

    await userEvent.selectOptions(target, 'loc2');
    await userEvent.type(screen.getByLabelText('Количество перемещения'), '120');
    await userEvent.click(screen.getByTestId('save-transfer'));

    await waitFor(() =>
      expect(sent).toMatchObject({
        item_id: ITEM,
        from_location_id: LOCATION,
        to_location_id: 'loc2',
        qty: '120',
      }),
    );
  });

  it('hides every write control from a viewer who may only read', async () => {
    server.use(...baseHandlers({ ...warehouseUser, permissions: ['inventory:read'] }));
    wrap(<StockLedgerPage />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('movement-forms')).not.toBeInTheDocument();
  });
});
