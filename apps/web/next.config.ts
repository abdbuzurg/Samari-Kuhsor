import path from 'node:path';

import type { NextConfig } from 'next';
import createNextIntlPlugin from 'next-intl/plugin';

// next-intl in ROUTED mode, the opposite of the CRM's choice
// (docs/07-IMPLEMENTATION-PLAN.md I14).
//
// The CRM is behind a login and stores locale in a cookie: a URL segment would
// buy nothing and cost thirteen modules an extra path level. The public site is
// the opposite case — search engines need a distinct, linkable URL per language
// and an hreflang set that points at real addresses, neither of which a cookie
// can provide.
const withNextIntl = createNextIntlPlugin('./i18n/request.ts');

const nextConfig: NextConfig = {
  // Lean production image: the container copies .next/standalone rather than the
  // whole node_modules tree (I18).
  output: 'standalone',
  outputFileTracingRoot: path.join(import.meta.dirname, '../../'),
  reactStrictMode: true,
  // Nothing is prefixed NEXT_PUBLIC_. BACKEND_URL and SERVICE_KEY are read only
  // inside app/api route handlers, which run on the server (CLAUDE.md §3).
  env: {},
};

export default withNextIntl(nextConfig);
