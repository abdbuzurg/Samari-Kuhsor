import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, afterAll, beforeAll, vi } from 'vitest';

import { server } from './msw';

/**
 * MSW intercepts the BFF routes so component tests drive real states by choosing
 * a handler rather than by hand-rolling a mock per component.
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

// next/navigation is aliased to a stub in vitest.config.ts — see the note there
// on why a vi.mock here would not reach next-intl's own import.

// next/link renders a plain anchor in tests; the real one needs a router.
vi.mock('next/link', () => ({
  default: ({ children, href, ...rest }: { children: React.ReactNode; href: string }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

// IntersectionObserver drives the assembly line's "start when visible" trigger.
// jsdom has none; the component falls back to rendering parked, which is the
// path these tests exercise. Defining a no-op stub here would HIDE that
// fallback, so it is deliberately left undefined.
