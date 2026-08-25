'use client';

import { useEffect } from 'react';
import { usePathname } from 'next/navigation';
import { useLocale } from 'next-intl';

import { track } from '@/lib/analytics';
import { useConsent } from '@/lib/consent';

/**
 * Page views, and the delegated link listener.
 *
 * Mounted once in the layout. Two jobs:
 *
 *   1. A `page_view` on every route change. Next's client-side navigation does
 *      not reload the document, so a load-time-only tracker would record the
 *      first page of a visit and nothing after it.
 *   2. One delegated click listener for every link on the site. Delegation
 *      rather than a hook per component: instrumenting each link individually is
 *      how half of them end up uninstrumented six months later.
 *
 * `product_view` is NOT emitted here. It fires only where a product is actually
 * shown — the product page and the belt modal — so the three routes to a product
 * (belt modal, catalogue card, search landing) each produce exactly one view.
 */
export function AnalyticsTracker() {
  const pathname = usePathname();
  const locale = useLocale();
  const consent = useConsent();

  useEffect(() => {
    if (consent !== 'granted' || !pathname) return;
    track({ kind: 'page_view', target: pathname, locale });
  }, [pathname, locale, consent]);

  useEffect(() => {
    if (consent !== 'granted') return;

    function onClick(e: MouseEvent) {
      const el = (e.target as HTMLElement | null)?.closest('a, button');
      if (!el) return;

      const anchor = el instanceof HTMLAnchorElement ? el : null;
      const href = anchor?.getAttribute('href') ?? el.getAttribute('data-sk-target');
      if (!href) return;

      // An explicit marker wins; otherwise position decides. Nav and footer are
      // captured but never shown on the dashboard — they always win on volume
      // and tell the owner nothing (D12).
      const marked = el.getAttribute('data-sk-category');
      const category =
        marked ??
        (el.closest('footer')
          ? 'footer'
          : el.closest('nav')
            ? 'nav'
            : /^https?:\/\//.test(href)
              ? 'outbound'
              : 'cta');

      track({
        kind: 'link_click',
        target: href,
        category: category as 'cta' | 'product' | 'nav' | 'footer' | 'outbound',
        locale,
        // Set on the modal's CTAs, so «Запросить цену» is attributable to the
        // product it was clicked from.
        sku: el.getAttribute('data-sk-sku') ?? undefined,
      });
    }

    document.addEventListener('click', onClick, { capture: true });
    return () => document.removeEventListener('click', onClick, { capture: true });
  }, [locale, consent]);

  return null;
}
