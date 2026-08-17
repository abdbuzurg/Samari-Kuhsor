'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';

/**
 * TanStack Query, the CRM's data layer (docs/07-IMPLEMENTATION-PLAN.md I9).
 *
 * Its query states map 1:1 onto the four states CLAUDE.md §7 requires every data
 * component to be tested in — isLoading / isError / empty / populated — which is
 * what makes that testing requirement mechanical rather than bespoke per module.
 */
export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Permissions must never be cached beyond the request
            // (docs/04-RBAC.md:148). Zero staleness is the safe default here;
            // individual queries opt into caching where it is harmless.
            staleTime: 0,
            retry: (failureCount, error) => {
              // Never retry an authorization failure: a 401 or 403 will not
              // become a 200 by asking again, and retrying just delays the
              // redirect to the login page.
              const status = (error as { status?: number }).status;
              if (status === 401 || status === 403) return false;
              return failureCount < 2;
            },
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
