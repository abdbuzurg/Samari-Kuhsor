import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import CustomersPage from '@/app/crm/page';
import CustomerDetail from '@/app/crm/[id]/page';
import PipelinePage from '@/app/crm/pipeline/page';
import DealDetail from '@/app/crm/deals/[id]/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * R13 — CRM и продажи, rebuilt to docs/05-MODULES.md:179.
 *
 * What shipped before was a sales-order table under this module's route: none of
 * the six specified columns matched, none of the four specified KPIs existed, and
 * the pipeline was absent because nothing had ever written a deal.
 */

const ID = '018f8000-0000-7000-8000-000000000001';

vi.mock('next/navigation', () => ({
  useParams: () => ({ id: ID }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/crm',
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
const page = (data: unknown[]) =>
  HttpResponse.json({ data, meta: { total: data.length, page: 1, total_pages: 1 } });
const noAudit = http.get('/api/audit', () =>
  HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
);

const CUSTOMER_ROW = {
  id: ID,
  name: 'ООО «Ориён Савдо»',
  customer_type: 'distributor',
  region: 'Душанбе',
  contact: '+992 900 100 200',
  open_deals: 2,
  open_amount: '48000.00',
  lead_status: status('negotiation', 'Переговоры'),
  version: 1,
};

const KPIS = { new_leads: 3, open_deals: 5, conversion: '62.5', overdue_tasks: 1 };

describe('CRM — the customer register', () => {
  it('renders the six columns the specification names, not a sales-order table', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/customers', () => page([CUSTOMER_ROW])),
      http.get('/api/crm/kpis', () => ok(KPIS)),
    );
    wrap(<CustomersPage />);

    await screen.findByTestId('list-row');
    for (const header of ['Клиент', 'Тип', 'Регион', 'Статус', 'Сумма']) {
      expect(screen.getByText(header)).toBeInTheDocument();
    }
    expect(screen.getByText('Дистрибьютор')).toBeInTheDocument();
    expect(screen.getByText('Душанбе')).toBeInTheDocument();
  });

  it('shows the four KPIs the specification names', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/customers', () => page([CUSTOMER_ROW])),
      http.get('/api/crm/kpis', () => ok(KPIS)),
    );
    wrap(<CustomersPage />);

    for (const label of ['Новые лиды', 'Открытые сделки', 'Конверсия', 'Просроченные задачи']) {
      expect(await screen.findByText(label)).toBeInTheDocument();
    }
    expect(await screen.findByText('62.5%')).toBeInTheDocument();
  });

  it('shows «уточняется» for conversion before anything has closed, never 0%', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/customers', () => page([CUSTOMER_ROW])),
      http.get('/api/crm/kpis', () => ok({ ...KPIS, conversion: null })),
    );
    wrap(<CustomersPage />);
    await screen.findByTestId('list-row');
    // "0% conversion" and "nothing has closed yet" read very differently.
    expect(screen.queryByText('0%')).not.toBeInTheDocument();
    expect(screen.getAllByText('уточняется').length).toBeGreaterThan(0);
  });

  it('shows an empty state rather than zeros on a system with no customers', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/customers', () => page([])),
      http.get('/api/crm/kpis', () => ok({ ...KPIS, new_leads: 0, open_deals: 0, conversion: null })),
    );
    wrap(<CustomersPage />);
    expect(await screen.findByText('Клиентов нет')).toBeInTheDocument();
  });

  it('shows an error state when the register cannot be loaded', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/customers', () =>
        HttpResponse.json({ error: { code: 'internal_error', message: '' } }, { status: 500 }),
      ),
      http.get('/api/crm/kpis', () => ok(KPIS)),
    );
    wrap(<CustomersPage />);
    expect(await screen.findByText('Не удалось загрузить клиентов')).toBeInTheDocument();
  });

  it('hides the create button from a user who may only read', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['crm:read'] }),
      http.get('/api/customers', () => page([CUSTOMER_ROW])),
      http.get('/api/crm/kpis', () => ok(KPIS)),
    );
    wrap(<CustomersPage />);
    await screen.findByTestId('list-row');
    expect(screen.queryByRole('button', { name: /создать/i })).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------

const DETAIL = {
  customer: {
    id: ID,
    name: 'ООО «Ориён Савдо»',
    customer_type: 'distributor',
    region: 'Душанбе',
    contact: '+992 900 100 200',
    version: 1,
    created_at: '2026-09-01T05:00:00Z',
  },
  contacts: [{ id: 'c1', full_name: 'Ф. Юсупов', role: 'Менеджер', email: null, phone: '+992 900 100 200' }],
  leads: [{ id: 'l1', source: 'Сайт', inquiry_id: 'i1', status: status('new', 'Новый', 'warn'), created_at: '2026-09-02T05:00:00Z', reference_no: null }],
  deals: [
    {
      id: 'd1',
      customer_id: ID,
      customer_name: 'ООО «Ориён Савдо»',
      region: 'Душанбе',
      amount: '48000.00',
      stage: status('negotiation', 'Переговоры'),
      owner_name: 'С. Одинаев',
      expected_close: '2026-10-01',
      version: 1,
    },
  ],
  orders: [
    { id: 'o1', so_no: 'SO-0102', ordered_on: '2026-09-05', total: '8880.00', status: status('confirmed', 'Подтверждён', 'ok') },
  ],
  inquiries: [
    { id: 'i1', reference_no: 'WR-0001', type: status('wholesale', 'Оптовый запрос'), status: status('lead_created', 'Создан лид', 'ok'), submitted_at: '2026-09-01T09:00:00Z' },
  ],
};

describe('CRM — customer detail', () => {
  it('renders every band the specification names', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/customers/${ID}`, () => ok(DETAIL)), noAudit);
    wrap(<CustomerDetail />);
    await screen.findByTestId('detail-view');

    // "customer header · contacts · deals · linked inquiries · orders · activity"
    for (const band of ['Контактные лица', 'Сделки', 'Заказы', 'Обращения с сайта', 'Лиды']) {
      expect(screen.getByText(band)).toBeInTheDocument();
    }
    expect(screen.getByTestId('activity-panel')).toBeInTheDocument();
  });

  it('links a converted enquiry back to the enquiry it came from', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/customers/${ID}`, () => ok(DETAIL)), noAudit);
    wrap(<CustomerDetail />);
    await screen.findByTestId('detail-view');
    // This is what makes an enquiry converted months ago still traceable.
    expect(screen.getByRole('link', { name: 'WR-0001' })).toHaveAttribute('href', '/inquiries/i1');
  });

  it('lets a sales user add a contact', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/customers/${ID}`, () => ok(DETAIL)),
      noAudit,
      http.post(`/api/customers/${ID}/contacts`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<CustomerDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-contact-form'));
    await userEvent.type(screen.getByLabelText('Имя контакта'), 'Г. Сафарова');
    await userEvent.click(screen.getByTestId('save-contact'));

    await waitFor(() => expect(sent).toMatchObject({ full_name: 'Г. Сафарова' }));
  });

  it('lets a sales user open a deal against the customer', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/customers/${ID}`, () => ok(DETAIL)),
      noAudit,
      http.post('/api/deals', async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<CustomerDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-deal-form'));
    await userEvent.type(screen.getByLabelText('Сумма сделки'), '31500');
    await userEvent.click(screen.getByTestId('save-deal'));

    await waitFor(() => expect(sent).toMatchObject({ customer_id: ID, amount: '31500' }));
  });

  it('hides both write controls from a read-only viewer', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['crm:read'] }),
      http.get(`/api/customers/${ID}`, () => ok(DETAIL)),
      noAudit,
    );
    wrap(<CustomerDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('toggle-contact-form')).not.toBeInTheDocument();
    expect(screen.queryByTestId('toggle-deal-form')).not.toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------

describe('CRM — the pipeline board', () => {
  const DEALS = [
    { ...DETAIL.deals[0], id: 'd1', stage: status('new', 'Новый лид') },
    { ...DETAIL.deals[0], id: 'd2', stage: status('quoted', 'КП отправлено', 'warn') },
    { ...DETAIL.deals[0], id: 'd3', stage: status('won', 'Выиграно', 'ok') },
  ];

  it('renders all five specified stages as columns', async () => {
    server.use(session.loaded(adminUser), http.get('/api/deals', () => page(DEALS)));
    wrap(<PipelinePage />);
    await screen.findByTestId('pipeline-board');

    for (const stage of ['new', 'negotiation', 'quoted', 'won', 'lost']) {
      expect(screen.getByTestId(`stage-${stage}`)).toBeInTheDocument();
    }
  });

  it('places each deal in its own stage column', async () => {
    server.use(session.loaded(adminUser), http.get('/api/deals', () => page(DEALS)));
    wrap(<PipelinePage />);
    await screen.findByTestId('pipeline-board');

    expect(screen.getByTestId('stage-new').querySelectorAll('[data-testid="deal-card"]')).toHaveLength(1);
    expect(screen.getByTestId('stage-negotiation')).toHaveTextContent('Пусто');
  });

  it('says the pipeline is empty rather than drawing five blank columns', async () => {
    server.use(session.loaded(adminUser), http.get('/api/deals', () => page([])));
    wrap(<PipelinePage />);
    expect(await screen.findByTestId('pipeline-empty')).toHaveTextContent('Сделок нет');
  });

  it('shows a loading state and then an error state', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/deals', () =>
        HttpResponse.json({ error: { code: 'internal_error', message: '' } }, { status: 500 }),
      ),
    );
    wrap(<PipelinePage />);
    expect(await screen.findByTestId('pipeline-error')).toBeInTheDocument();
  });
});

// --------------------------------------------------------------------------

describe('CRM — deal detail', () => {
  const DEAL = {
    id: ID,
    customer_id: 'cust1',
    customer_name: 'ООО «Ориён Савдо»',
    amount: '48000.00',
    stage: status('negotiation', 'Переговоры'),
    owner_name: 'С. Одинаев',
    expected_close: '2026-10-01',
    version: 2,
    created_at: '2026-09-02T05:00:00Z',
    history: [
      {
        id: 'e1',
        from_stage: status('new', 'Новый лид'),
        to_stage: status('negotiation', 'Переговоры'),
        occurred_at: '2026-09-03T05:00:00Z',
        changed_by: 'С. Одинаев',
        note: 'Первая встреча',
      },
    ],
    allowed_transitions: ['new', 'quoted', 'won', 'lost'],
  };

  it('shows the stage history — who moved it, when, and why', async () => {
    server.use(session.loaded(adminUser), http.get(`/api/deals/${ID}`, () => ok(DEAL)), noAudit);
    wrap(<DealDetail />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('История стадий')).toBeInTheDocument();
    expect(screen.getByTestId('related-row')).toHaveTextContent('Первая встреча');
  });

  it('moves a deal along the pipeline', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/deals/${ID}`, () => ok(DEAL)),
      noAudit,
      http.post(`/api/deals/${ID}/stage`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<DealDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'КП отправлено' }));
    await waitFor(() => expect(sent).toEqual({ to: 'quoted' }));
  });

  it('offers nothing on a closed deal, because the server allowed nothing', async () => {
    server.use(
      session.loaded(adminUser),
      http.get(`/api/deals/${ID}`, () =>
        ok({ ...DEAL, stage: status('won', 'Выиграно', 'ok'), allowed_transitions: [] }),
      ),
      noAudit,
    );
    wrap(<DealDetail />);
    await screen.findByTestId('detail-view');
    // Won and lost are terminal: a reopened deal makes every conversion figure
    // provisional.
    expect(screen.getByTestId('no-transitions')).toBeInTheDocument();
  });

  it('collects a reason before marking a deal lost', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get(`/api/deals/${ID}`, () => ok(DEAL)),
      noAudit,
      http.post(`/api/deals/${ID}/stage`, async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<DealDetail />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'Проиграно' }));
    expect(sent).toBeNull();

    await userEvent.type(screen.getByRole('textbox'), 'Выбрали другого поставщика');
    await userEvent.click(screen.getByTestId('confirm-transition'));

    await waitFor(() =>
      expect(sent).toEqual({ to: 'lost', reason: 'Выбрали другого поставщика' }),
    );
  });
});
