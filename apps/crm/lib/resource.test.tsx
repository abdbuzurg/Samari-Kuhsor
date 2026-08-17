import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import type { ReactNode } from 'react';

import { createResourceHooks, toSearchParams, orTBC, TBC } from '@/lib/resource';
import { ApiError } from '@/lib/session';
import { server } from '@/test/msw';

/**
 * The extracted engine (docs/07-IMPLEMENTATION-PLAN.md I2).
 *
 * Eleven modules will depend on this, so it is tested directly rather than only
 * through Товары. A bug here is a bug in every module at once.
 */

interface Row {
  id: string;
  name: string;
}
interface Detail extends Row {
  version: number;
}

const widgets = createResourceHooks<Row, Detail>('widgets');

let client: QueryClient;

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
});

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe('toSearchParams', () => {
  it('omits defaults so the URL stays readable and cacheable', () => {
    expect(toSearchParams({})).toBe('');
    // Page 1 is the default; sending it would make the first page a different
    // cache key from the same page reached by paging back.
    expect(toSearchParams({ page: 1 })).toBe('');
  });

  it('includes search, paging, sort, locale and module filters', () => {
    const s = toSearchParams({
      q: 'сок',
      page: 3,
      sort: '-created_at',
      locale: 'tg',
      filters: { status: 'active', item_type: undefined },
    });
    const params = new URLSearchParams(s.slice(1));
    expect(params.get('q')).toBe('сок');
    expect(params.get('page')).toBe('3');
    expect(params.get('sort')).toBe('-created_at');
    expect(params.get('locale')).toBe('tg');
    expect(params.get('status')).toBe('active');
    // An undefined filter is omitted, not sent as the string "undefined".
    expect(params.has('item_type')).toBe(false);
  });
});

describe('createResourceHooks', () => {
  it('derives the URL and the cache key from the same resource name', async () => {
    let requested = '';
    server.use(
      http.get('/api/widgets', ({ request }) => {
        requested = new URL(request.url).pathname;
        return HttpResponse.json({
          data: [{ id: '1', name: 'A' }],
          meta: { page: 1, per_page: 50, total: 1, total_pages: 1 },
        });
      }),
    );

    const { result } = renderHook(() => widgets.useList({}), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(requested).toBe('/api/widgets');
    expect(result.current.data?.data).toHaveLength(1);
    expect(result.current.data?.meta.total).toBe(1);
  });

  // A collection response keeps `meta` alongside `data`; losing it breaks paging.
  it('returns meta alongside data', async () => {
    server.use(
      http.get('/api/widgets', () =>
        HttpResponse.json({
          data: [],
          meta: { page: 2, per_page: 25, total: 60, total_pages: 3 },
        }),
      ),
    );
    const { result } = renderHook(() => widgets.useList({ page: 2 }), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.meta).toEqual({ page: 2, per_page: 25, total: 60, total_pages: 3 });
  });

  it('surfaces the API error code, not just a failure', async () => {
    server.use(
      http.get('/api/widgets', () =>
        HttpResponse.json(
          { error: { code: 'forbidden', message: 'Недостаточно прав' } },
          { status: 403 },
        ),
      ),
    );
    const { result } = renderHook(() => widgets.useList({}), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    const error = result.current.error as ApiError;
    expect(error).toBeInstanceOf(ApiError);
    expect(error.code).toBe('forbidden');
    expect(error.status).toBe(403);
  });

  it('carries per-field details through so a form can place them', async () => {
    server.use(
      http.post('/api/widgets', () =>
        HttpResponse.json(
          {
            error: {
              code: 'validation_failed',
              message: 'Проверьте заполненные поля',
              details: [{ field: 'sku', code: 'already_exists', message: 'SKU уже используется' }],
            },
          },
          { status: 400 },
        ),
      ),
    );

    const { result } = renderHook(() => widgets.useCreate(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({}).catch(() => {});
    });
    // The mutation's error settles on the next render, so wait for it rather
    // than reading it straight after the await.
    await waitFor(() => expect(result.current.isError).toBe(true));

    const error = result.current.error as ApiError;
    expect(error.code).toBe('validation_failed');
    expect(error.forField('sku')?.code).toBe('already_exists');
    expect(error.forField('nonexistent')).toBeUndefined();
  });

  /**
   * The subtle one. After a save, the detail cache must hold the version the
   * SERVER just wrote — not the stale one the form still has.
   *
   * Invalidating alone leaves a window in which a second save sends the old
   * version and 409s against the user's own previous save, which looks like the
   * app randomly refusing to save.
   */
  it('seeds the detail cache from the update response so the next save has the new version', async () => {
    let patches = 0;
    server.use(
      http.get('/api/widgets/w1', () =>
        HttpResponse.json({ data: { id: 'w1', name: 'A', version: 3 } }),
      ),
      http.patch('/api/widgets/w1', async ({ request }) => {
        patches++;
        const body = (await request.json()) as { version: number };
        if (body.version !== 2 + patches) {
          return HttpResponse.json(
            { error: { code: 'version_conflict', message: 'Конфликт версий' } },
            { status: 409 },
          );
        }
        return HttpResponse.json({ data: { id: 'w1', name: 'B', version: body.version + 1 } });
      }),
    );

    const { result } = renderHook(
      () => ({ one: widgets.useOne('w1'), update: widgets.useUpdate('w1') }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.one.isSuccess).toBe(true));
    expect(result.current.one.data?.version).toBe(3);

    await act(async () => {
      await result.current.update.mutateAsync({ version: 3 });
    });

    // Without the setQueryData seed this would still read 3, and the next save
    // would send a stale version.
    await waitFor(() => expect(result.current.one.data?.version).toBe(4));

    // Prove it: a second save using the cached version succeeds.
    await act(async () => {
      await result.current.update.mutateAsync({ version: result.current.one.data!.version });
    });
    expect(result.current.update.isError).toBe(false);
  });

  it('sends the version when tombstoning', async () => {
    let sent: unknown = null;
    server.use(
      http.delete('/api/widgets/w1', async ({ request }) => {
        sent = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const { result } = renderHook(() => widgets.useRemove('w1'), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(7);
    });

    // Deleting a record someone else just edited must fail the same way editing
    // it would, so the tombstone carries a version too.
    expect(sent).toEqual({ version: 7 });
  });

  it('posts to a sub-resource for actions', async () => {
    let path = '';
    server.use(
      http.post('/api/widgets/w1/release', ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.json({ data: { ok: true } });
      }),
    );

    const { result } = renderHook(() => widgets.useAction('w1', 'release'), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ reason: 'passed' });
    });

    // State transitions are sub-resources, never a PATCH of a status field
    // (docs/03-API-CONTRACT.md:74) — that is what keeps permissions and audit
    // entries precise.
    expect(path).toBe('/api/widgets/w1/release');
  });

  it('does not fetch a detail until an id exists', () => {
    const { result } = renderHook(() => widgets.useOne(undefined), { wrapper });
    expect(result.current.fetchStatus).toBe('idle');
  });
});

describe('orTBC', () => {
  // docs/02-SCHEMA.md:176 — an empty cell reads as "none"; «уточняется» reads as
  // "not yet determined", which is the truth until the lab confirms it.
  it('renders the placeholder for anything absent', () => {
    for (const absent of [null, undefined, '']) {
      expect(orTBC(absent)).toBe(TBC);
    }
  });

  it('passes real values through, including zero', () => {
    expect(orTBC('стекло')).toBe('стекло');
    // Zero is a value, not an absence: a min_qty of 0 means "alert at zero",
    // which is different from "no threshold set".
    expect(orTBC(0)).toBe('0');
  });
});
