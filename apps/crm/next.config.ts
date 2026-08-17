import path from 'node:path';

import type { NextConfig } from 'next';
import createNextIntlPlugin from 'next-intl/plugin';

// next-intl in UNROUTED mode: the CRM is behind a login, so locale is a user
// preference in a cookie, not a URL segment (docs/07-IMPLEMENTATION-PLAN.md I14).
// apps/web uses the routed mode instead, because there SEO needs per-language URLs.
const withNextIntl = createNextIntlPlugin('./i18n/request.ts');

const nextConfig: NextConfig = {
  // Lean production image: the container copies .next/standalone rather than
  // the whole node_modules tree (I18).
  output: 'standalone',
  outputFileTracingRoot: path.join(import.meta.dirname, '../../'),
  reactStrictMode: true,
  // The browser must never learn the backend's address. BACKEND_URL and
  // SERVICE_KEY are read only inside app/api route handlers, which run on the
  // server; nothing here is prefixed NEXT_PUBLIC_ (CLAUDE.md §3).
  env: {},
};

export default withNextIntl(nextConfig);
