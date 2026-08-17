'use client';

import Script from 'next/script';

import { useConsent } from '@/lib/consent';

/**
 * Matomo, self-hosted (docs/01-DECISIONS.md — no third-party analytics).
 *
 * The script tag is not rendered at all until consent is granted. That is
 * stronger than the usual pattern of loading the tracker and setting an opt-out
 * flag: nothing is requested from the analytics host, so there is no request log
 * to explain and declining leaves no trace.
 *
 * If the environment does not name a Matomo host, nothing renders regardless.
 * The client tests over a bare IP before analytics exists, and a broken script
 * tag pointing at an unset host would be a console error on every page.
 */
export function Analytics() {
  const consent = useConsent();
  const host = process.env.NEXT_PUBLIC_MATOMO_URL;
  const siteId = process.env.NEXT_PUBLIC_MATOMO_SITE_ID;

  if (consent !== 'granted' || !host || !siteId) return null;

  const base = host.replace(/\/+$/, '');

  return (
    <Script id="matomo" strategy="afterInteractive" data-testid="matomo">
      {`
        var _paq = window._paq = window._paq || [];
        _paq.push(['trackPageView']);
        _paq.push(['enableLinkTracking']);
        (function() {
          var u = ${JSON.stringify(base + '/')};
          _paq.push(['setTrackerUrl', u + 'matomo.php']);
          _paq.push(['setSiteId', ${JSON.stringify(siteId)}]);
          var d = document, g = d.createElement('script'), s = d.getElementsByTagName('script')[0];
          g.async = true; g.src = u + 'matomo.js'; s.parentNode.insertBefore(g, s);
        })();
      `}
    </Script>
  );
}
