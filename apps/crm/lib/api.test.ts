import { describe, it, expect, vi, afterEach } from 'vitest';

// lib/api.ts is server-only by design: importing it from a client component must
// fail the build. That guard has no meaning in a test runner, so it is stubbed
// here — the module's behaviour is still what is under test.
vi.mock('next/headers', () => ({
  cookies: async () => ({ get: () => undefined }),
}));

// Set before the import: lib/api.ts reads these at module scope, which is right
// for a server process whose env is fixed at boot, but means beforeEach is too
// late.
process.env.BACKEND_URL = 'http://api.test';
process.env.SERVICE_KEY = 'test-key';

const { callApi, relay } = await import('@/lib/api');

/**
 * The BFF proxy layer.
 *
 * This file exists because a real bug shipped past the component tests: they mock
 * `/api/items` directly, so nothing exercised the BFF, and `relay()` was dropping
 * `meta` from every collection response. Pagination and every total in the UI
 * were broken while every test stayed green.
 *
 * The lesson is the layer, not the field: mocking at the browser boundary leaves
 * the BFF untested, so the BFF needs tests of its own.
 */

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
  vi.restoreAllMocks();
});

function mockFetch(status: number, body: unknown) {
  const spy = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
    status === 204
      ? new Response(null, { status })
      : new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
  );
  globalThis.fetch = spy as unknown as typeof fetch;
  return spy;
}

describe('callApi', () => {
  // docs/03-API-CONTRACT.md §4 — a collection response is {data, meta}. Losing
  // meta breaks paging and every total the UI shows.
  it('carries meta through from a collection response', async () => {
    mockFetch(200, {
      data: [{ id: '1' }],
      meta: { page: 1, per_page: 50, total: 212, total_pages: 5 },
    });

    const result = await callApi('/items');
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.meta).toEqual({ page: 1, per_page: 50, total: 212, total_pages: 5 });
  });

  it('attaches the service key but never a user identity', async () => {
    const spy = mockFetch(200, { data: {} });
    await callApi('/items');

    const init = spy.mock.calls[0]?.[1] as RequestInit | undefined;
    const headers = (init?.headers ?? {}) as Record<string, string>;
    expect(headers['X-Service-Key']).toBe('test-key');
    // docs/07-IMPLEMENTATION-PLAN.md I8 — the BFF must never send a user id.
    // That would move identity resolution out of Go and make the permission
    // system forgeable by anything that reaches the API port.
    for (const header of Object.keys(headers)) {
      expect(header.toLowerCase()).not.toContain('user');
    }
  });

  it('returns the API error envelope unchanged', async () => {
    mockFetch(400, {
      error: {
        code: 'validation_failed',
        message: 'Проверьте заполненные поля',
        details: [{ field: 'sku', code: 'already_exists', message: 'SKU уже используется' }],
      },
    });

    const result = await callApi('/items');
    expect(result.ok).toBe(false);
    if (result.ok) return;
    // The BFF does not reshape errors: the frontend switches on error.code
    // (docs/03-API-CONTRACT.md:116), so translating them here would break every
    // consumer for no gain.
    expect(result.status).toBe(400);
    expect(result.error.code).toBe('validation_failed');
    expect(result.error.details?.[0].field).toBe('sku');
  });

  it('handles 204 without trying to parse a body', async () => {
    mockFetch(204, null);
    const result = await callApi('/items/x');
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.status).toBe(204);
  });

  // The Go API being unreachable is an infrastructure fault. It must arrive in
  // the contract's own error shape so the client has one thing to parse — and it
  // must never leak the backend address.
  it('reports an unreachable backend in the contract shape, without leaking its address', async () => {
    globalThis.fetch = vi.fn(async () => {
      throw new Error('connect ECONNREFUSED http://api.test:8080');
    }) as unknown as typeof fetch;
    vi.spyOn(console, 'error').mockImplementation(() => {});

    const result = await callApi('/items');
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.status).toBe(503);
    expect(result.error.code).toBe('internal_error');
    expect(JSON.stringify(result.error)).not.toContain('api.test');
    expect(JSON.stringify(result.error)).not.toContain('ECONNREFUSED');
  });
});

describe('relay', () => {
  it('preserves meta on a collection response', async () => {
    const res = relay({
      ok: true,
      status: 200,
      data: [{ id: '1' }],
      meta: { page: 2, per_page: 50, total: 212, total_pages: 5 },
    });
    const body = await res.json();

    expect(body.data).toHaveLength(1);
    // The regression this file was written for.
    expect(body.meta).toEqual({ page: 2, per_page: 50, total: 212, total_pages: 5 });
  });

  it('omits meta entirely for a single-record response', async () => {
    const res = relay({ ok: true, status: 200, data: { id: '1' } });
    const body = await res.json();
    expect(body).toEqual({ data: { id: '1' } });
    expect('meta' in body).toBe(false);
  });

  it('preserves the status and the error envelope', async () => {
    const res = relay({
      ok: false,
      status: 409,
      error: { code: 'version_conflict', message: 'Конфликт версий' },
    });
    expect(res.status).toBe(409);
    expect((await res.json()).error.code).toBe('version_conflict');
  });

  // Per-user, permission-filtered data must never be cached by an intermediary.
  it('marks every response no-store', () => {
    const res = relay({ ok: true, status: 200, data: {} });
    expect(res.headers.get('Cache-Control')).toBe('no-store');
  });
});
