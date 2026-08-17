import { vi } from 'vitest';

/**
 * A stand-in for `next/navigation` under Vitest.
 *
 * Aliased in vitest.config.ts rather than mocked in setup, because next-intl's
 * client navigation imports the module from inside node_modules — a vi.mock in
 * the test file only intercepts the test's own import specifier, not that one.
 */
export const useRouter = () => ({
  push: vi.fn(),
  replace: vi.fn(),
  refresh: vi.fn(),
  prefetch: vi.fn(),
  back: vi.fn(),
  forward: vi.fn(),
});

export const usePathname = () => '/';
export const useSearchParams = () => new URLSearchParams();
export const useParams = () => ({ locale: 'ru' });
export const useSelectedLayoutSegment = () => null;
export const useSelectedLayoutSegments = () => [];
export const redirect = vi.fn();
export const permanentRedirect = vi.fn();
export function notFound(): never {
  throw new Error('NEXT_NOT_FOUND');
}
export const RedirectType = { push: 'push', replace: 'replace' } as const;
