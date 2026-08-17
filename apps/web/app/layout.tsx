import type { ReactNode } from 'react';

import './globals.css';

/**
 * The root layout exists only to satisfy Next's requirement for one.
 *
 * Everything real — <html lang>, the chrome, the fonts — lives in
 * app/[locale]/layout.tsx, because none of it can be decided before the locale
 * is known.
 */
export default function RootLayout({ children }: { children: ReactNode }) {
  return children;
}
