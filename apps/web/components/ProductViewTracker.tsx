'use client';

import { useEffect } from 'react';
import { useLocale } from 'next-intl';

import { track } from '@/lib/analytics';
import { useConsent } from '@/lib/consent';

/**
 * One `product_view`, from wherever a product is actually shown.
 *
 * Rendered by the product page and by the belt modal — the only two surfaces
 * that display a product. Deliberately NOT rendered by the catalogue card or the
 * hero list: those are links, and the destination emits the view. Counting the
 * click as well would double every catalogue-routed product against every
 * belt-routed one and invert the ranking (docs/01-DECISIONS.md D12).
 *
 * The effect keys on the SKU, so opening two products in the modal without
 * closing it in between records two views rather than one.
 */
export function ProductViewTracker({
  sku,
  source,
}: {
  sku: string;
  source: 'product_page' | 'belt_modal';
}) {
  const locale = useLocale();
  const consent = useConsent();

  useEffect(() => {
    if (consent !== 'granted' || !sku) return;
    track({ kind: 'product_view', target: sku, source, locale });
  }, [sku, source, locale, consent]);

  return null;
}
