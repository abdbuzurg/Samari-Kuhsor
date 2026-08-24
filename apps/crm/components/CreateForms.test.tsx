import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import NewBatchPage from '@/app/quality/new/page';
import NewEmployeePage from '@/app/hr/new/page';
import NewDocumentPage from '@/app/documents/new/page';
import NewPurchaseOrderPage from '@/app/procurement/new/page';
import TasksPage from '@/app/crm/tasks/page';
import { server, session, adminUser, warehouseUser } from '@/test/msw';
import messages from '@/messages/ru.json';

/**
 * The create forms.
 *
 * These were the last unreachable write paths: ten `useCreate*` hooks defined,
 * wired and called by nothing — the same defect the whole recovery plan started
 * from, one layer down.
 */

const push = vi.fn();

vi.mock('next/navigation', () => ({
  useParams: () => ({}),
  useRouter: () => ({ push, replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}));

let client: QueryClient;
beforeEach(() => {
  push.mockClear();
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
});

function wrap(node: ReactNode) {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </NextIntlClientProvider>,
  );
}

const ok = (data: unknown) => HttpResponse.json({ data });
const page = (data: unknown[]) =>
  HttpResponse.json({ data, meta: { total: data.length, page: 1, total_pages: 1 } });

const ITEMS = [
  { id: 'i1', sku: 'APJ-1000', name: 'Яблочный сок' },
  { id: 'i2', sku: 'WAT-500', name: 'Вода 0,5 л' },
];

describe('Новая партия', () => {
  it('creates a batch and lands on it', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get('/api/items', () => page(ITEMS)),
      http.post('/api/batches', async ({ request }) => {
        sent = await request.json();
        return ok({ id: 'b-new' });
      }),
    );
    wrap(<NewBatchPage />);

    await screen.findByTestId('create-form');
    await userEvent.type(screen.getByLabelText('Номер партии'), 'B-2620');
    await userEvent.selectOptions(await screen.findByLabelText('Товар'), 'i2');
    await userEvent.click(screen.getByTestId('create-save'));

    await waitFor(() => expect(sent).toMatchObject({ batch_no: 'B-2620', item_id: 'i2' }));
    // Landing on the record it just made — a create that returns to a list
    // makes the user hunt for their own row.
    await waitFor(() => expect(push).toHaveBeenCalledWith('/quality/b-new'));
  });

  it('will not submit without the required fields', async () => {
    server.use(session.loaded(adminUser), http.get('/api/items', () => page(ITEMS)));
    wrap(<NewBatchPage />);
    await screen.findByTestId('create-form');
    // Номер партии is empty.
    expect(screen.getByTestId('create-save')).toBeDisabled();
  });

  it('shows the server’s validation message against the form', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/items', () => page(ITEMS)),
      http.post('/api/batches', () =>
        HttpResponse.json(
          { error: { code: 'validation_failed', message: 'Партия с таким номером уже есть' } },
          { status: 422 },
        ),
      ),
    );
    wrap(<NewBatchPage />);

    await screen.findByTestId('create-form');
    await userEvent.type(screen.getByLabelText('Номер партии'), 'B-2617');
    await userEvent.selectOptions(await screen.findByLabelText('Товар'), 'i1');
    await userEvent.click(screen.getByTestId('create-save'));

    expect(await screen.findByTestId('create-error')).toHaveTextContent(
      'Партия с таким номером уже есть',
    );
  });
});

describe('Новый сотрудник', () => {
  it('sends the Russian labels’ underlying keys, not the labels', async () => {
    let sent: { shift?: string } | null = null;
    server.use(
      session.loaded(adminUser),
      http.post('/api/employees', async ({ request }) => {
        sent = (await request.json()) as { shift?: string };
        return ok({ id: 'e-new' });
      }),
    );
    wrap(<NewEmployeePage />);

    await screen.findByTestId('create-form');
    await userEvent.type(screen.getByLabelText('ФИО'), 'М. Назарова');
    await userEvent.selectOptions(screen.getByLabelText('Смена'), 'night');
    await userEvent.click(screen.getByTestId('create-save'));

    // The backend stores keys; the reader gets Russian (C3).
    await waitFor(() => expect(sent?.shift).toBe('night'));
  });
});

describe('Новый документ', () => {
  it('creates a draft — the status is never chosen on the form', async () => {
    let sent: Record<string, unknown> | null = null;
    server.use(
      session.loaded(adminUser),
      http.post('/api/documents', async ({ request }) => {
        sent = (await request.json()) as Record<string, unknown>;
        return ok({ id: 'd-new' });
      }),
    );
    wrap(<NewDocumentPage />);

    await screen.findByTestId('create-form');
    await userEvent.type(screen.getByLabelText('Номер'), 'DOC-009');
    await userEvent.type(screen.getByLabelText('Название'), 'Регламент мойки');
    await userEvent.click(screen.getByTestId('create-save'));

    await waitFor(() => expect(sent).toBeTruthy());
    // A status dropdown here would bypass the approval ladder it exists to
    // enforce: `active` needs documents:approve.
    expect(sent).not.toHaveProperty('status');
  });
});

describe('Новый заказ поставщику', () => {
  const SUPPLIERS = [{ id: 's1', name: 'Памир Агро', region: null, contact: null, tax_id: null, version: 1 }];

  it('will not submit without at least one complete line', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/suppliers', () => page(SUPPLIERS)),
      http.get('/api/items', () => page(ITEMS)),
    );
    wrap(<NewPurchaseOrderPage />);

    await screen.findByTestId('po-form');
    await userEvent.type(screen.getByLabelText('Номер заказа'), 'PO-0040');
    await userEvent.selectOptions(await screen.findByLabelText('Поставщик'), 's1');
    // The one line is empty.
    expect(screen.getByTestId('po-save')).toBeDisabled();
  });

  it('sends the header and its lines together', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get('/api/suppliers', () => page(SUPPLIERS)),
      http.get('/api/items', () => page(ITEMS)),
      http.post('/api/purchase-orders', async ({ request }) => {
        sent = await request.json();
        return ok({ id: 'po-new' });
      }),
    );
    wrap(<NewPurchaseOrderPage />);

    await screen.findByTestId('po-form');
    await userEvent.type(screen.getByLabelText('Номер заказа'), 'PO-0040');
    await userEvent.selectOptions(await screen.findByLabelText('Поставщик'), 's1');
    await userEvent.selectOptions(await screen.findByLabelText('Позиция 1'), 'i1');
    await userEvent.type(screen.getByLabelText('Количество 1'), '1000');
    await userEvent.type(screen.getByLabelText('Цена 1'), '15.00');
    await userEvent.click(screen.getByTestId('po-save'));

    await waitFor(() =>
      expect(sent).toMatchObject({
        po_no: 'PO-0040',
        supplier_id: 's1',
        lines: [{ item_id: 'i1', qty: '1000', unit_price: '15.00' }],
      }),
    );
  });

  it('adds a second line', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/suppliers', () => page(SUPPLIERS)),
      http.get('/api/items', () => page(ITEMS)),
    );
    wrap(<NewPurchaseOrderPage />);

    await screen.findByTestId('po-form');
    expect(screen.getAllByTestId('po-line')).toHaveLength(1);
    await userEvent.click(screen.getByTestId('add-line'));
    expect(screen.getAllByTestId('po-line')).toHaveLength(2);
  });
});

describe('Задачи', () => {
  const TASK = {
    id: 't1',
    title: 'Перезвонить в «Ориён Савдо»',
    assignee_name: 'С. Одинаев',
    due_on: '2020-01-01',
    status: { key: 'open', label: 'Открыта', level: 'info' },
    related_type: null,
    related_id: null,
    version: 1,
  };

  it('flags an overdue task, because that is what the KPI counts', async () => {
    server.use(session.loaded(adminUser), http.get('/api/tasks', () => page([TASK])));
    wrap(<TasksPage />);

    await screen.findByTestId('list-row');
    // Overdue is a property of the date, not a status anybody sets.
    expect(screen.getByText('2020-01-01').tagName).toBe('STRONG');
  });

  it('closes a task', async () => {
    let sent: unknown = null;
    server.use(
      session.loaded(adminUser),
      http.get('/api/tasks', () => page([TASK])),
      http.put('/api/tasks/t1/status', async ({ request }) => {
        sent = await request.json();
        return ok({});
      }),
    );
    wrap(<TasksPage />);

    await screen.findByTestId('list-row');
    await userEvent.click(screen.getByTestId('close-task'));
    await waitFor(() => expect(sent).toEqual({ status: 'done' }));
  });

  it('hides the write controls from a read-only viewer', async () => {
    server.use(
      session.loaded({ ...warehouseUser, permissions: ['crm:read'] }),
      http.get('/api/tasks', () => page([TASK])),
    );
    wrap(<TasksPage />);
    await screen.findByTestId('list-row');
    expect(screen.queryByTestId('close-task')).not.toBeInTheDocument();
  });
});
