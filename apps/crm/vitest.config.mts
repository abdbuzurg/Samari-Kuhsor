import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./test/setup.ts'],
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['node_modules/**', '.next/**'],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
      '@samari/types': path.resolve(__dirname, '../../packages/types/api.ts'),
      // `server-only` is a Next build-time guard with no runtime module Vite can
      // resolve. Stubbing it here lets the BFF be tested; the guard still fails
      // the real build if a client component imports lib/api.ts.
      'server-only': path.resolve(__dirname, 'test/server-only-stub.ts'),
    },
  },
});
