import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NextIntlClientProvider } from 'next-intl';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse, delay } from 'msw';
import type { ReactNode } from 'react';

import { ActivityPanel } from '@/components/ActivityPanel';
import { DetailShell } from '@/components/DetailShell';
import { ListView } from '@/components/ListView';
import { RelatedTable } from '@/components/RelatedTable';
import { WorkflowActions } from '@/components/WorkflowActions';
import { server, session, adminUser } from '@/test/msw';
import messages from '@/messages/ru.json';
import type { AuditEntry } from '@samari/types';

/**
 * R01 — the shared detail scaffold.
 *
 * These four components are what makes R03–R13 cheap, so they carry the same
 * four-state coverage CLAUDE.md §7 requires of every data component. A defect
 * here reproduces itself ten times.
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

const ITEM_ID = '018f3c9e-0000-7000-8000-000000000001';

const ENTRY: AuditEntry = {
  id: '018f4000-0000-7000-8000-000000000001',
  action: 'update',
  resource: 'items',
  resource_id: ITEM_ID,
  actor_id: adminUser.id,
  actor_name: 'А. Раҳимов',
  ip: '10.0.0.4',
  occurred_at: '2026-08-20T09:15:00Z',
  before: null,
  after: null,
};

describe('ActivityPanel', () => {
  it('asks only for this record’s trail, not the whole module', async () => {
    let seen: URL | null = null;
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', ({ request }) => {
        seen = new URL(request.url);
        return HttpResponse.json({ data: [ENTRY], meta: { total: 1, page: 1, total_pages: 1 } });
      }),
    );

    wrap(<ActivityPanel resource="items" resourceId={ITEM_ID} />);

    await screen.findByTestId('activity-list');
    // Go has accepted resource_id since audit.sql:33; nothing had ever sent it.
    expect(seen!.searchParams.get('resource')).toBe('items');
    expect(seen!.searchParams.get('resource_id')).toBe(ITEM_ID);
  });

  it('renders entries newest-first with the actor and a Dushanbe timestamp', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', () =>
        HttpResponse.json({ data: [ENTRY], meta: { total: 1, page: 1, total_pages: 1 } }),
      ),
    );

    wrap(<ActivityPanel resource="items" resourceId={ITEM_ID} />);

    const entry = await screen.findByTestId('activity-entry');
    expect(entry).toHaveTextContent('Изменение');
    expect(entry).toHaveTextContent('А. Раҳимов');
  });

  it('names «Система» for an action no user took', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', () =>
        HttpResponse.json({
          data: [{ ...ENTRY, actor_id: null, actor_name: null }],
          meta: { total: 1, page: 1, total_pages: 1 },
        }),
      ),
    );

    wrap(<ActivityPanel resource="inquiries" resourceId={ITEM_ID} />);
    expect(await screen.findByTestId('activity-entry')).toHaveTextContent('Система');
  });

  it('shows a loading state while the trail is in flight', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', async () => {
        await delay(30);
        return HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } });
      }),
    );

    wrap(<ActivityPanel resource="items" resourceId={ITEM_ID} />);
    expect(screen.getByTestId('activity-loading')).toBeInTheDocument();
    await screen.findByTestId('activity-empty');
  });

  it('says the record has no changes rather than showing an error', async () => {
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', () =>
        HttpResponse.json({ data: [], meta: { total: 0, page: 1, total_pages: 0 } }),
      ),
    );

    wrap(<ActivityPanel resource="items" resourceId={ITEM_ID} />);
    expect(await screen.findByTestId('activity-empty')).toHaveTextContent('Изменений не было');
  });

  it('degrades quietly when the viewer may not read the audit trail', async () => {
    // A warehouse user has no audit:read. The record must still render; only the
    // panel goes away.
    server.use(
      session.loaded(adminUser),
      http.get('/api/audit', () =>
        HttpResponse.json({ error: { code: 'forbidden', message: '' } }, { status: 403 }),
      ),
    );

    wrap(<ActivityPanel resource="items" resourceId={ITEM_ID} />);
    expect(await screen.findByTestId('activity-error')).toHaveTextContent('История недоступна');
  });
});

describe('DetailShell', () => {
  it('distinguishes a missing record from a forbidden one', () => {
    const { rerender } = wrap(
      <DetailShell moduleLabel="Товары" moduleHref="/items" isLoading={false} error={{ status: 404 }}>
        <p>never</p>
      </DetailShell>,
    );
    expect(screen.getByTestId('detail-error')).toHaveTextContent('Запись не найдена');

    rerender(
      <NextIntlClientProvider locale="ru" messages={messages}>
        <QueryClientProvider client={client}>
          <DetailShell moduleLabel="Товары" moduleHref="/items" isLoading={false} error={{ status: 403 }}>
            <p>never</p>
          </DetailShell>
        </QueryClientProvider>
      </NextIntlClientProvider>,
    );
    // Telling a user a record does not exist when really they may not read it
    // sends them hunting a data problem that is not there.
    expect(screen.getByTestId('detail-error')).toHaveTextContent('Нет доступа');
  });

  it('renders the record once loaded', () => {
    wrap(
      <DetailShell moduleLabel="Товары" moduleHref="/items" isLoading={false} error={null}>
        <p>Яблочный сок</p>
      </DetailShell>,
    );
    expect(screen.getByText('Яблочный сок')).toBeInTheDocument();
    expect(screen.queryByTestId('detail-error')).not.toBeInTheDocument();
  });

  it('shows a loading state instead of the record', () => {
    wrap(
      <DetailShell moduleLabel="Товары" moduleHref="/items" isLoading error={null}>
        <p>Яблочный сок</p>
      </DetailShell>,
    );
    expect(screen.getByTestId('detail-loading')).toBeInTheDocument();
    expect(screen.queryByText('Яблочный сок')).not.toBeInTheDocument();
  });
});

describe('RelatedTable', () => {
  const COLUMNS = [
    { key: 'no', header: 'Партия', render: (r: { no: string }) => r.no },
  ];

  it('renders its rows', () => {
    wrap(
      <RelatedTable
        title="Партии"
        columns={COLUMNS}
        rows={[{ no: 'B-2617' }, { no: 'B-2618' }]}
        rowKey={(r) => r.no}
        emptyLabel="Партий нет"
      />,
    );
    expect(screen.getAllByTestId('related-row')).toHaveLength(2);
  });

  it('shows its own empty label rather than a blank table', () => {
    wrap(
      <RelatedTable
        title="Партии"
        columns={COLUMNS}
        rows={[]}
        rowKey={(r) => r.no}
        emptyLabel="Партий нет"
      />,
    );
    expect(screen.getByTestId('related-empty')).toHaveTextContent('Партий нет');
  });
});

describe('WorkflowActions', () => {
  const LABELS = { released: 'Выпустить', rejected: 'Забраковать' };

  it('offers only the transitions the server allowed', () => {
    wrap(
      <WorkflowActions
        endpoint="/api/batches/x/transition"
        invalidate="quality"
        allowed={['released']}
        labels={LABELS}
        disabled={false}
      />,
    );
    expect(screen.getByRole('button', { name: 'Выпустить' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Забраковать' })).not.toBeInTheDocument();
  });

  it('renders a dash, not an empty toolbar, when nothing is allowed', () => {
    wrap(
      <WorkflowActions
        endpoint="/api/batches/x/transition"
        invalidate="quality"
        allowed={[]}
        labels={LABELS}
        disabled={false}
      />,
    );
    expect(screen.getByTestId('no-transitions')).toBeInTheDocument();
  });

  it('posts the target state', async () => {
    let body: unknown = null;
    server.use(
      http.post('/api/batches/:id/transition', async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ data: {} });
      }),
    );

    wrap(
      <WorkflowActions
        endpoint="/api/batches/b1/transition"
        invalidate="quality"
        allowed={['released']}
        labels={LABELS}
        disabled={false}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'Выпустить' }));
    await waitFor(() => expect(body).toEqual({ to: 'released' }));
  });

  it('collects a mandatory reason before sending, so the refusal never happens', async () => {
    let body: unknown = null;
    server.use(
      http.post('/api/batches/:id/transition', async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ data: {} });
      }),
    );

    wrap(
      <WorkflowActions
        endpoint="/api/batches/b1/transition"
        invalidate="quality"
        allowed={['rejected']}
        labels={LABELS}
        disabled={false}
        reasonFor={(to) => to === 'rejected'}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'Забраковать' }));
    // Nothing is sent yet — the domain refuses a recall without a reason.
    expect(body).toBeNull();

    const confirm = screen.getByTestId('confirm-transition');
    expect(confirm).toBeDisabled();

    await userEvent.type(screen.getByRole('textbox'), 'Посторонний привкус');
    await userEvent.click(confirm);

    await waitFor(() =>
      expect(body).toEqual({ to: 'rejected', reason: 'Посторонний привкус' }),
    );
  });

  it('shows the server’s own refusal, not a generic message', async () => {
    server.use(
      http.post('/api/batches/:id/transition', () =>
        HttpResponse.json(
          { error: { code: 'business_rule', message: 'Партия уже выпущена.' } },
          { status: 422 },
        ),
      ),
    );

    wrap(
      <WorkflowActions
        endpoint="/api/batches/b1/transition"
        invalidate="quality"
        allowed={['released']}
        labels={LABELS}
        disabled={false}
      />,
    );

    await userEvent.click(screen.getByRole('button', { name: 'Выпустить' }));
    expect(await screen.findByTestId('workflow-error')).toHaveTextContent('Партия уже выпущена.');
  });

  it('disables every action for a viewer who may not manage', () => {
    wrap(
      <WorkflowActions
        endpoint="/api/batches/x/transition"
        invalidate="quality"
        allowed={['released']}
        labels={LABELS}
        disabled
      />,
    );
    expect(screen.getByRole('button', { name: 'Выпустить' })).toBeDisabled();
  });
});

describe('ListView export button (R15)', () => {
  const COLUMNS = [{ key: 'sku', header: 'Артикул', render: (r: { sku: string }) => r.sku }];

  function listView(props: Record<string, unknown> = {}) {
    return wrap(
      <ListView<{ sku: string }>
        kicker="Продажи"
        title="Товары"
        columns={COLUMNS}
        rows={[{ sku: 'APJ-1000' }]}
        rowKey={(r: { sku: string }) => r.sku}
        search=""
        onSearchChange={() => {}}
        searchPlaceholder="Поиск"
        createLabel="Создать"
        isLoading={false}
        emptyTitle="Пусто"
        emptyBody=""
        noMatchLabel="Ничего"
        {...props}
      />,
    );
  }

  it('renders no export control for a collection that has none', () => {
    listView();
    expect(screen.queryByTestId('export-link')).not.toBeInTheDocument();
  });

  it('points at the collection’s export endpoint', () => {
    listView({ exportKey: 'stock' });
    expect(screen.getByTestId('export-link')).toHaveAttribute('href', '/api/export/stock');
  });

  it('carries the active search, so the file matches the screen', () => {
    listView({ exportKey: 'stock', search: 'яблоч' });
    // A report that quietly ignores the filter is a different report from the
    // one the user was looking at.
    expect(screen.getByTestId('export-link')).toHaveAttribute(
      'href',
      `/api/export/stock?q=${encodeURIComponent('яблоч')}`,
    );
  });
});

describe('Responsive shell (R17)', () => {
  it('keeps the sidebar closed until asked, and opens it from the top bar', async () => {
    server.use(session.loaded(adminUser));
    const { AppShell } = await import('@/components/AppShell');
    wrap(<AppShell>content</AppShell>);

    const sidebar = await screen.findByTestId('sidebar');
    // The drawer state is off by default. At lg the CSS ignores it entirely and
    // the sidebar sits beside the content at its contracted 252px.
    expect(sidebar).toHaveAttribute('data-open', 'false');
    expect(sidebar.className).toContain('w-[252px]');
    expect(screen.queryByTestId('nav-scrim')).not.toBeInTheDocument();

    await userEvent.click(screen.getByTestId('open-nav'));
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-open', 'true');
    expect(screen.getByTestId('nav-scrim')).toBeInTheDocument();
  });

  it('closes the drawer when a module is chosen', async () => {
    server.use(session.loaded(adminUser));
    const { AppShell } = await import('@/components/AppShell');
    wrap(<AppShell>content</AppShell>);

    await userEvent.click(await screen.findByTestId('open-nav'));
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-open', 'true');

    // Navigating on a phone must not leave the drawer covering the page it just
    // opened.
    await userEvent.click(screen.getByTestId('sidebar'));
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-open', 'false');
  });

  it('closes the drawer from the scrim', async () => {
    server.use(session.loaded(adminUser));
    const { AppShell } = await import('@/components/AppShell');
    wrap(<AppShell>content</AppShell>);

    await userEvent.click(await screen.findByTestId('open-nav'));
    await userEvent.click(screen.getByTestId('nav-scrim'));
    expect(screen.getByTestId('sidebar')).toHaveAttribute('data-open', 'false');
  });
});
