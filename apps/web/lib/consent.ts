'use client';

import { useEffect, useState } from 'react';

/**
 * Analytics consent.
 *
 * Stored in localStorage rather than a cookie, deliberately: a cookie would be
 * sent on every request to our own origin for a preference the server never
 * reads, and a consent mechanism that itself creates server-visible state is a
 * poor argument to make to a regulator.
 *
 * Three states, not two. `unknown` is not `denied` — the banner must show once
 * and then stop showing, and collapsing "not asked" into "said no" would either
 * re-ask forever or treat silence as refusal without recording it.
 */
export type Consent = 'unknown' | 'granted' | 'denied';

const KEY = 'samari_analytics_consent';

export function readConsent(): Consent {
  if (typeof window === 'undefined') return 'unknown';
  try {
    const value = window.localStorage.getItem(KEY);
    return value === 'granted' || value === 'denied' ? value : 'unknown';
  } catch {
    // Private browsing can throw on access. Treat it as "not asked" rather than
    // crashing the page over an analytics preference.
    return 'unknown';
  }
}

export function writeConsent(value: Exclude<Consent, 'unknown'>): void {
  try {
    window.localStorage.setItem(KEY, value);
  } catch {
    // Nothing to do. The banner will show again next visit, which is the
    // correct failure mode: it errs towards asking rather than towards tracking.
  }
  window.dispatchEvent(new CustomEvent('samari:consent', { detail: value }));
}

/** Subscribes to the consent value, including changes made in this tab. */
export function useConsent(): Consent {
  // Starts as 'unknown' on both server and client so hydration matches; the
  // effect below reads the real value immediately after mount.
  const [consent, setConsent] = useState<Consent>('unknown');

  useEffect(() => {
    setConsent(readConsent());
    const onChange = () => setConsent(readConsent());
    window.addEventListener('samari:consent', onChange);
    window.addEventListener('storage', onChange);
    return () => {
      window.removeEventListener('samari:consent', onChange);
      window.removeEventListener('storage', onChange);
    };
  }, []);

  return consent;
}
