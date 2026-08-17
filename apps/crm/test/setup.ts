import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, afterAll, beforeAll, vi } from 'vitest';

import { server } from './msw';

/**
 * MSW intercepts the BFF routes so component tests drive the four states
 * CLAUDE.md §7 requires — loading, empty, error, populated — by choosing a
 * handler rather than by hand-rolling a mock per component.
 *
 * onUnhandledRequest: 'error' is deliberate. A component that quietly calls an
 * endpoint nobody stubbed would otherwise pass its test while being broken.
 */
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => {
  server.resetHandlers();
  cleanup();
});
afterAll(() => server.close());

// next/navigation is not available outside a Next runtime.
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}));

// Server actions cannot run in jsdom.
vi.mock('@/app/actions', () => ({ setLocale: vi.fn(async () => {}) }));
