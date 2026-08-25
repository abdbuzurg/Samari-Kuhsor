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

/**
 * The consent version.
 *
 * Bumped when WHAT IS COLLECTED changes, never when the wording is tidied
 * (docs/01-DECISIONS.md D12). A stored answer given against an older description
 * is not consent for a newer one, so anyone holding v1 — which described Matomo
 * and "which pages are useful" — is asked again against v2, which describes
 * product views, link clicks and a visit identifier.
 *
 * Fixing a typo must not re-prompt the country.
 */
export const CONSENT_VERSION = 2;

const KEY = 'samari_analytics_consent';

export function readConsent(): Consent {
  if (typeof window === 'undefined') return 'unknown';
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return 'unknown';

    // v1 stored a bare string. Anyone holding one consented to a different
    // description of a different tracker, so it is treated as never asked.
    if (raw === 'granted' || raw === 'denied') return 'unknown';

    const parsed = JSON.parse(raw) as { value?: unknown; version?: unknown };
    if (parsed?.version !== CONSENT_VERSION) return 'unknown';
    return parsed.value === 'granted' || parsed.value === 'denied'
      ? (parsed.value as Consent)
      : 'unknown';
  } catch {
    // Private browsing can throw on access. Treat it as "not asked" rather than
    // crashing the page over an analytics preference.
    return 'unknown';
  }
}

export function writeConsent(value: Exclude<Consent, 'unknown'>): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify({ value, version: CONSENT_VERSION }));
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

/**
 * Clears the stored answer so the banner asks again.
 *
 * Reachable from the privacy policy. Until D12 there was no path back at all:
 * decline once and the banner never returned, which meant a visitor could not
 * change their mind and a regulator would have nothing to look at.
 */
export function resetConsent(): void {
  try {
    window.localStorage.removeItem(KEY);
  } catch {
    // Nothing to do; the banner's absence is already the failure mode.
  }
  window.dispatchEvent(new CustomEvent('samari:consent', { detail: 'unknown' }));
}
