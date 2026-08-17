import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./test/setup.tsx'],
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['node_modules/**', '.next/**'],
    server: {
      deps: {
        // next-intl must be transformed rather than externalised, so the
        // next/navigation alias below reaches its internal import. Externalised
        // dependencies are loaded by Node directly and never see Vite's
        // resolver, which is why the alias alone was not enough.
        inline: ['next-intl'],
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
      '@samari/types': path.resolve(__dirname, '../../packages/types/api.ts'),
      // `server-only` is a Next build-time guard with no runtime module Vite can
      // resolve. Stubbing it here lets the BFF be tested; the guard still fails
      // the real build if a client component imports lib/api.ts.
      'server-only': path.resolve(__dirname, 'test/server-only-stub.ts'),
      // next-intl's client navigation imports next/navigation from inside
      // node_modules, which a vi.mock in a test file cannot intercept. Aliasing
      // it here catches every importer.
      'next/navigation': path.resolve(__dirname, 'test/next-navigation-stub.tsx'),
    },
  },
});
