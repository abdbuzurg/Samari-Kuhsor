import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import BatchDetailPage from '@/app/quality/[id]/page';
import { server, session, qcTechnician, qcLead } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { BatchDetail } from '@samari/types';

/**
 * R03 — Качество, the release screen.
 *
 * ToR §8 acceptance condition 5: "Quality staff can quarantine and release
 * finished goods." Before R03 there was no client code for this at all — not a
 * page, not a hook. These tests are written to the R-gate rule: each names a
 * role, an action and a browser, rather than asserting something about Go.
 */

const BATCH_ID = '018f5000-0000-7000-8000-000000000001';

// Replaces the global mock in test/setup.ts, so it must carry every hook the
// chrome uses as well — AppShell calls useRouter and Sidebar calls usePathname.
// Dropping either takes down the whole shell, not just this page.
vi.mock('next/navigation', () => ({
  useParams: () => ({ id: BATCH_ID }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/quality',
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

const QUARANTINED: BatchDetail = {
  batch: {
    id: BATCH_ID,
    batch_no: 'B-2617',
    item_id: '018f3c9e-0000-7000-8000-000000000001',
    produced_on: '2026-09-10',
    expires_on: '2027-09-10',
    qr_payload: null,
    qr_issued_at: null,
    status: { key: 'quarantine', label: 'Карантин', level: 'warn' },
    version: 2,
    created_at: '2026-09-10T06:00:00Z',
  },
  sku: 'APJ-1000',
  item_name: 'Яблочный сок прямого отжима',
  tests: [
    {
      id: 't1',
      batch_id: BATCH_ID,
      test_type: 'ph',
      result: { key: 'passed', label: 'Пройдено', level: 'ok' },
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
      from_status: { key: 'in_production', label: 'В производстве', level: 'info' },
      to_status: { key: 'quarantine', label: 'Карантин', level: 'warn' },
      occurred_at: '2026-09-10T07:00:00Z',
      decided_by: 'u1',
      decider_name: 'Система',
      reason: null,
    },
  ],
  stock: [
    {
      item_id: '018f3c9e-0000-7000-8000-000000000001',
      sku: 'APJ-1000',
      item_name: 'Яблочный сок прямого отжима',
      base_uom: 'bottle',
      batch_id: BATCH_ID,
      batch_no: 'B-2617',
      batch_status: { key: 'quarantine', label: 'Карантин', level: 'warn' },
      expires_on: '2027-09-10',
      location_id: 'l1',
      location_code: 'QRN-01',
      location_zone: 'Карантин',
      on_hand: '4800.000',
      min_qty: null,
      status: { key: 'ok', label: 'В норме', level: 'ok' },
      last_movement_at: '2026-09-10T07:00:00Z',
    },
  ],
  // What the SERVER says this user may do. quality:approve holders get both.
  allowed_transitions: ['released', 'rejected'],
};

const RELEASED: BatchDetail = {
  ...QUARANTINED,
  batch: { ...QUARANTINED.batch, status: { key: 'released', label: 'Выпущена', level: 'ok' } },
  // A recall is the only move left once a batch is out.
  allowed_transitions: ['rejected'],
};

function serveBatch(detail: BatchDetail = QUARANTINED) {
  return http.get(`/api/batches/${BATCH_ID}/detail`, () => HttpResponse.json({ data: detail }));
}

const noAudit = http.get('/api/audit', () =>
  HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
);

describe('Качество — batch detail', () => {
  it('shows the batch, its tests, its decision history and where the stock is', async () => {
    server.use(session.loaded(qcLead), serveBatch(), noAudit);
    wrap(<BatchDetailPage />);

    await screen.findByTestId('detail-view');
    // Once as the title, once in the field group.
    expect(screen.getAllByText('Яблочный сок прямого отжима').length).toBeGreaterThan(0);
    expect(screen.getByText(/B-2617 · APJ-1000/)).toBeInTheDocument();

    // The three related bands: tests, decisions, stock.
    const tables = screen.getAllByTestId('related-table');
    expect(tables).toHaveLength(3);
    expect(screen.getByText('М. Назарова')).toBeInTheDocument();
    expect(screen.getByText('QRN-01 · Карантин')).toBeInTheDocument();
    expect(screen.getByText(/4800\.000/)).toBeInTheDocument();
  });

  it('shows a loading state while the batch is in flight', async () => {
    server.use(
      session.loaded(qcLead),
      http.get(`/api/batches/${BATCH_ID}/detail`, async () => {
        await delay(30);
        return HttpResponse.json({ data: QUARANTINED });
      }),
      noAudit,
    );
    wrap(<BatchDetailPage />);
    expect(await screen.findByTestId('detail-loading')).toBeInTheDocument();
    await screen.findByTestId('detail-view');
  });

  it('says the batch was not found rather than showing an empty record', async () => {
    server.use(
      session.loaded(qcLead),
      http.get(`/api/batches/${BATCH_ID}/detail`, () =>
        HttpResponse.json({ error: { code: 'not_found', message: '' } }, { status: 404 }),
      ),
      noAudit,
    );
    wrap(<BatchDetailPage />);
    expect(await screen.findByTestId('detail-error')).toHaveTextContent('Запись не найдена');
  });

  it('renders «уточняется» for a batch nobody has tested yet', async () => {
    server.use(
      session.loaded(qcLead),
      serveBatch({ ...QUARANTINED, tests: [] }),
      noAudit,
    );
    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');
    expect(screen.getByText('Проверок ещё не было.')).toBeInTheDocument();
  });
});

describe('Качество — release', () => {
  it('lets a user holding quality:approve release a quarantined batch from the browser', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(qcLead),
      serveBatch(),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/transition`, async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ data: RELEASED.batch });
      }),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'Выпустить' }));
    await waitFor(() => expect(sent).toEqual({ to: 'released' }));
  });

  it('offers a technician no release button, because the server allowed none', async () => {
    server.use(
      session.loaded(qcTechnician),
      // quality:manage without approve — the server returns an empty list, and
      // the screen must not invent one. AllowedFrom is the only authority.
      serveBatch({ ...QUARANTINED, allowed_transitions: [] }),
      noAudit,
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    expect(screen.queryByRole('button', { name: 'Выпустить' })).not.toBeInTheDocument();
    expect(screen.getByTestId('no-transitions')).toBeInTheDocument();
  });

  it('collects a reason before a recall, and sends it', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(qcLead),
      serveBatch(RELEASED),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/transition`, async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ data: RELEASED.batch });
      }),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'Забраковать' }));
    // released → rejected is a recall; the domain refuses it without a reason
    // (quality.go:78), so nothing may be sent yet.
    expect(sent).toBeNull();

    await userEvent.type(screen.getByRole('textbox'), 'Посторонний привкус');
    await userEvent.click(screen.getByTestId('confirm-transition'));

    await waitFor(() =>
      expect(sent).toEqual({ to: 'rejected', reason: 'Посторонний привкус' }),
    );
  });

  it('does not demand a reason for an ordinary release', async () => {
    server.use(
      session.loaded(qcLead),
      serveBatch(),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/transition`, () =>
        HttpResponse.json({ data: RELEASED.batch }),
      ),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'Выпустить' }));
    expect(screen.queryByTestId('transition-reason')).not.toBeInTheDocument();
  });

  it('shows the server’s refusal verbatim when the move is rejected', async () => {
    server.use(
      session.loaded(qcLead),
      serveBatch(),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/transition`, () =>
        HttpResponse.json(
          { error: { code: 'forbidden', message: 'Требуется разрешение quality:approve.' } },
          { status: 403 },
        ),
      ),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByRole('button', { name: 'Выпустить' }));
    expect(await screen.findByTestId('workflow-error')).toHaveTextContent(
      'Требуется разрешение quality:approve.',
    );
  });
});

describe('Качество — recording a test', () => {
  it('lets a technician record a laboratory result', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(qcTechnician),
      serveBatch({ ...QUARANTINED, allowed_transitions: [] }),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/tests`, async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ data: {} });
      }),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-test-form'));
    await userEvent.selectOptions(screen.getByLabelText('Тип проверки'), 'microbiology');
    await userEvent.type(screen.getByLabelText('Значение'), '<10 КОЕ/г');
    await userEvent.click(screen.getByTestId('save-test'));

    await waitFor(() =>
      expect(sent).toEqual({
        test_type: 'microbiology',
        result_value: '<10 КОЕ/г',
        passed: true,
        notes: undefined,
      }),
    );
  });

  it('records a failed result when the inspector unticks «пройдена»', async () => {
    let sent: { passed?: boolean } | null = null;
    server.use(
      session.loaded(qcTechnician),
      serveBatch({ ...QUARANTINED, allowed_transitions: [] }),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/tests`, async ({ request }) => {
        sent = (await request.json()) as { passed?: boolean };
        return HttpResponse.json({ data: {} });
      }),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-test-form'));
    await userEvent.click(screen.getByLabelText('Проверка пройдена'));
    await userEvent.click(screen.getByTestId('save-test'));

    await waitFor(() => expect(sent?.passed).toBe(false));
  });

  it('hides the recorder from a viewer who may only read', async () => {
    server.use(
      session.loaded({ ...qcTechnician, permissions: ['quality:read'] }),
      serveBatch({ ...QUARANTINED, allowed_transitions: [] }),
      noAudit,
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');
    expect(screen.queryByTestId('toggle-test-form')).not.toBeInTheDocument();
  });

  it('surfaces a validation failure against the form', async () => {
    server.use(
      session.loaded(qcTechnician),
      serveBatch({ ...QUARANTINED, allowed_transitions: [] }),
      noAudit,
      http.post(`/api/batches/${BATCH_ID}/tests`, () =>
        HttpResponse.json(
          { error: { code: 'validation_failed', message: 'Неизвестный тип проверки' } },
          { status: 422 },
        ),
      ),
    );

    wrap(<BatchDetailPage />);
    await screen.findByTestId('detail-view');

    await userEvent.click(screen.getByTestId('toggle-test-form'));
    await userEvent.click(screen.getByTestId('save-test'));

    expect(await screen.findByTestId('test-error')).toHaveTextContent('Неизвестный тип проверки');
  });
});
