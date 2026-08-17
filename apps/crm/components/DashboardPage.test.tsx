import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import DashboardPage from '@/app/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { Dashboard } from '@samari/types';

/**
 * Панель управления.
 *
 * The two failures worth testing are not layout. They are:
 *   1. showing a figure the viewer is not entitled to, on the one screen they
 *      cannot avoid opening;
 *   2. showing an invented figure — the prototype's sample data — for a factory
 *      that has produced nothing.
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

/** An opening-day dashboard: everything the viewer may read, all of it zero. */
const EMPTY: Dashboard = {
  period: 'month',
  sales: { revenue: '0.00', order_count: 0, open_orders: 0, overdue_purchase_orders: 0 },
  stock: { value: '0.00', below_minimum: 0 },
  quality: { quarantined: 0 },
  production: { good_qty: '0.000', scrap_qty: '0.000', planned_qty: '0.000', progress: 0 },
  pipeline: [],
  recent_orders: [],
  feed: [],
  revenue: Array.from({ length: 30 }, (_, i) => ({
    day: `2026-07-${String(i + 1).padStart(2, '0')}`,
    revenue: '0.00',
    order_count: 0,
  })),
};

const RUNNING: Dashboard = {
  ...EMPTY,
  sales: { revenue: '48200.00', order_count: 12, open_orders: 3, overdue_purchase_orders: 2 },
  stock: { value: '184300.00', below_minimum: 1 },
  quality: { quarantined: 2 },
  production: {
    good_qty: '4800.000',
    scrap_qty: '120.000',
    planned_qty: '5000.000',
    progress: 96,
  },
  recent_orders: [
    {
      id: '018f3c9e-0000-7000-8000-000000000001',
      so_no: 'SO-000021',
      customer_name: 'Маркет Хорог',
      total: '12400.00',
      status: { key: 'confirmed', label: 'Подтверждён', level: 'info' },
      created_at: '2026-08-16T06:00:00Z',
    },
  ],
  feed: [
    {
      id: '018f3c9e-0000-7000-8000-000000000002',
      action: 'approve',
      resource: 'quality',
      resource_id: '018f3c9e-0000-7000-8000-000000000003',
      actor_name: 'Ф. Давлатова',
      occurred_at: '2026-08-17T09:00:00Z',
    },
  ],
  revenue: EMPTY.revenue.map((p, i) => ({
    ...p,
    revenue: i === 29 ? '48200.00' : '0.00',
  })),
};

function dashboard(payload: Dashboard) {
  return http.get('/api/dashboard', () => HttpResponse.json({ data: payload }));
}

// ---------------------------------------------------------------------------
// The four states
// ---------------------------------------------------------------------------

it('shows a loading state while the request is in flight', async () => {
  server.use(
    session.loaded(adminUser),
    http.get('/api/dashboard', async () => {
      await delay('infinite');
      return HttpResponse.json({ data: EMPTY });
    }),
  );
  wrap(<DashboardPage />);
  await waitFor(() => {
    expect(screen.getByTestId('dashboard-loading')).toBeInTheDocument();
  });
});

it('shows an error state when the request fails', async () => {
  server.use(
    session.loaded(adminUser),
    http.get('/api/dashboard', () =>
      HttpResponse.json(
        { error: { code: 'internal_error', message: 'Внутренняя ошибка сервера' } },
        { status: 500 },
      ),
    ),
  );
  wrap(<DashboardPage />);
  expect(await screen.findByTestId('dashboard-error')).toBeInTheDocument();
});

it('renders the panels once loaded', async () => {
  server.use(session.loaded(adminUser), dashboard(RUNNING));
  wrap(<DashboardPage />);
  expect(await screen.findByText('48200.00 с.')).toBeInTheDocument();
  expect(screen.getByText('SO-000021')).toBeInTheDocument();
});

// ---------------------------------------------------------------------------
// No fabricated figures
// ---------------------------------------------------------------------------

describe('an empty system', () => {
  it('renders zero rather than the prototype sample figures', async () => {
    server.use(session.loaded(adminUser), dashboard(EMPTY));
    wrap(<DashboardPage />);

    // Revenue and stock value both read 0.00 с., which is the point.
    await waitFor(() => {
      expect(screen.getAllByText('0.00 с.').length).toBeGreaterThan(0);
    });
    // The prototype's headline number. If it ever appears, someone has carried
    // sample data into production (05-MODULES.md:70).
    expect(screen.queryByText(/2 480 000/)).not.toBeInTheDocument();
    expect(screen.queryByText(/2480000/)).not.toBeInTheDocument();
  });

  it('says so in words rather than drawing a flat line at zero', async () => {
    server.use(session.loaded(adminUser), dashboard(EMPTY));
    wrap(<DashboardPage />);
    await screen.findByText('Продаж за выбранный период пока нет.');
    // A sparkline of thirty zeroes is a horizontal rule, which reads as a chart
    // that failed to load rather than as "no sales yet".
    expect(screen.queryByTestId('sparkline')).not.toBeInTheDocument();
  });

  it('shows an empty feed and an empty order list as such', async () => {
    server.use(session.loaded(adminUser), dashboard(EMPTY));
    wrap(<DashboardPage />);
    await screen.findByText('Заказов ещё не было.');
    expect(screen.getByText('Событий пока нет.')).toBeInTheDocument();
    expect(screen.queryByTestId('recent-order')).not.toBeInTheDocument();
  });

  it('draws the sparkline once there is something to draw', async () => {
    server.use(session.loaded(adminUser), dashboard(RUNNING));
    wrap(<DashboardPage />);
    expect(await screen.findByTestId('sparkline')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// No leaked panels
// ---------------------------------------------------------------------------

describe('permission filtering', () => {
  // The server sends null for a module the viewer may not read. Null and 0 look
  // identical rendered, so the panel must be omitted, not zeroed.
  const WAREHOUSE_ONLY: Dashboard = {
    ...EMPTY,
    sales: null,
    quality: null,
    production: null,
    stock: { value: '184300.00', below_minimum: 1 },
  };

  it('omits a panel the server sent as null, rather than rendering zero', async () => {
    server.use(session.loaded(warehouseUser), dashboard(WAREHOUSE_ONLY));
    wrap(<DashboardPage />);

    expect(await screen.findByText('184300.00 с.')).toBeInTheDocument();

    const strip = screen.getByTestId('kpi-strip');
    expect(within(strip).queryByText(messages.kpi.rev)).not.toBeInTheDocument();
    expect(within(strip).queryByText(messages.kpi.qc)).not.toBeInTheDocument();
    // And no revenue panel at all — not an empty one.
    expect(screen.queryByLabelText(messages.dash.rev)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(messages.dash.orders)).not.toBeInTheDocument();
  });

  it('renders the panel when the module IS readable, so the omission is meaningful', async () => {
    server.use(session.loaded(adminUser), dashboard(RUNNING));
    wrap(<DashboardPage />);
    await screen.findByText('48200.00 с.');
    expect(screen.getByLabelText(messages.dash.rev)).toBeInTheDocument();
    expect(screen.getByLabelText(messages.dash.orders)).toBeInTheDocument();
  });

  it('shows the event feed to everyone — the server has already filtered it', async () => {
    server.use(session.loaded(warehouseUser), dashboard(WAREHOUSE_ONLY));
    wrap(<DashboardPage />);
    await screen.findByText('184300.00 с.');
    // The feed panel exists; its contents are scoped in Go to the viewer's
    // readable resources, so the frontend does not filter it a second time.
    expect(screen.getByLabelText(messages.dash.feed)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Behaviour
// ---------------------------------------------------------------------------

describe('period switcher', () => {
  it('refetches with the chosen period', async () => {
    const asked: string[] = [];
    server.use(
      session.loaded(adminUser),
      http.get('/api/dashboard', ({ request }) => {
        asked.push(new URL(request.url).searchParams.get('period') ?? '');
        return HttpResponse.json({ data: EMPTY });
      }),
    );
    wrap(<DashboardPage />);
    await screen.findByText('Продаж за выбранный период пока нет.');
    expect(asked).toContain('month');

    await userEvent.click(screen.getByRole('button', { name: messages.period.week }));
    await waitFor(() => {
      expect(asked).toContain('week');
    });
  });

  it('marks the active period for assistive technology', async () => {
    server.use(session.loaded(adminUser), dashboard(EMPTY));
    wrap(<DashboardPage />);
    await screen.findByText('Продаж за выбранный период пока нет.');
    expect(screen.getByRole('button', { name: messages.period.month })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByRole('button', { name: messages.period.day })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });
});

describe('the event feed', () => {
  it('renders the audit keys in Russian rather than showing them raw', async () => {
    server.use(session.loaded(adminUser), dashboard(RUNNING));
    wrap(<DashboardPage />);
    const entry = await screen.findByTestId('feed-entry');
    // The server sends `approve` and `quality` as keys so the frontend can render
    // them in the reader's locale (docs/07 C3).
    expect(entry).toHaveTextContent('согласовал партию');
    expect(entry).toHaveTextContent('Ф. Давлатова');
    expect(entry).not.toHaveTextContent('approve');
  });

  it('attributes an actor-less entry to the system rather than leaving it blank', async () => {
    const systemEvent: Dashboard = {
      ...RUNNING,
      feed: [{ ...RUNNING.feed[0], actor_name: '', action: 'create', resource: 'inquiries' }],
    };
    server.use(session.loaded(adminUser), dashboard(systemEvent));
    wrap(<DashboardPage />);
    const entry = await screen.findByTestId('feed-entry');
    // A public website submission has no user behind it. "Система создал
    // обращение" is clumsy but truthful; a blank name reads as a bug.
    expect(entry).toHaveTextContent('Система');
  });
});

describe('problem counts', () => {
  it('never renders a count of problems in green', async () => {
    server.use(session.loaded(adminUser), dashboard(RUNNING));
    wrap(<DashboardPage />);
    await screen.findByText('48200.00 с.');

    // Green means healthy inside the content area (CLAUDE.md §5), so a count of
    // quarantined batches is red or plain — never the brand colour.
    const strip = screen.getByTestId('kpi-strip');
    const qc = within(strip).getByText(messages.kpi.qc).closest('[data-testid="kpi"]');
    const value = qc?.querySelector('div:nth-child(2)') as HTMLElement;
    expect(value.style.color).toContain('danger');
  });

  it('leaves a zero count uncoloured', async () => {
    server.use(session.loaded(adminUser), dashboard(EMPTY));
    wrap(<DashboardPage />);
    await screen.findByText('Продаж за выбранный период пока нет.');

    const strip = screen.getByTestId('kpi-strip');
    const qc = within(strip).getByText(messages.kpi.qc).closest('[data-testid="kpi"]');
    const value = qc?.querySelector('div:nth-child(2)') as HTMLElement;
    expect(value.style.color).toBe('');
  });
});
