'use client';

import { readConsent } from '@/lib/consent';

/**
 * First-party website statistics (docs/01-DECISIONS.md D12).
 *
 * Replaces Matomo. Three properties are load-bearing and none is incidental:
 *
 *   - NOTHING is sent before consent. Not queued-then-discarded: `track()`
 *     returns immediately when consent is not 'granted', so there is no buffer
 *     holding a declining visitor's behaviour in memory either.
 *   - Identity is a SESSION, not a visitor. A random id in sessionStorage, gone
 *     when the tab closes, never written to localStorage or a cookie.
 *   - Events are BATCHED and flushed with sendBeacon, which is the only
 *     transport that survives a tab close — and a tab close is exactly when the
 *     most interesting click happens.
 */

export type EventKind = 'page_view' | 'product_view' | 'link_click';
export type LinkCategory = 'cta' | 'product' | 'nav' | 'footer' | 'outbound';

export interface TrackedEvent {
  kind: EventKind;
  target: string;
  source?: 'product_page' | 'belt_modal';
  category?: LinkCategory;
  locale: string;
  sku?: string;
}

const SESSION_KEY = 'samari_analytics_session';
const ENDPOINT = '/api/analytics';

/** Flush triggers. 25 keeps a batch under the server's 50 cap with room spare. */
const MAX_BUFFER = 25;
const FLUSH_MS = 15_000;

let buffer: TrackedEvent[] = [];
let timer: ReturnType<typeof setTimeout> | null = null;
let listenersBound = false;

/**
 * The visit id.
 *
 * sessionStorage rather than localStorage: the design is that this disappears
 * when the tab closes. crypto.randomUUID where available — a Math.random id
 * would collide often enough at low traffic to merge two visitors into one
 * "visit", which is the single number the owner's ranking is built on.
 */
function sessionId(): string | null {
  try {
    const existing = window.sessionStorage.getItem(SESSION_KEY);
    if (existing) return existing;
    const id =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `s-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
    window.sessionStorage.setItem(SESSION_KEY, id);
    return id;
  } catch {
    // Private browsing can throw. No session means no events, which is the
    // correct failure mode: it errs towards collecting nothing.
    return null;
  }
}

/** Queues one event. A no-op unless consent has been granted. */
export function track(event: TrackedEvent): void {
  if (typeof window === 'undefined') return;
  if (readConsent() !== 'granted') return;

  buffer.push(event);
  bindListeners();

  if (buffer.length >= MAX_BUFFER) {
    flush();
    return;
  }
  if (timer === null) {
    timer = setTimeout(flush, FLUSH_MS);
  }
}

/**
 * Sends whatever is buffered.
 *
 * sendBeacon first: it is queued by the browser and survives the page going
 * away, which fetch() does not. fetch with keepalive is the fallback for the
 * handful of browsers without it.
 */
export function flush(): void {
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
  if (buffer.length === 0) return;
  if (readConsent() !== 'granted') {
    // Consent was withdrawn between queueing and flushing. Drop it.
    buffer = [];
    return;
  }

  const id = sessionId();
  if (!id) {
    buffer = [];
    return;
  }

  const payload = JSON.stringify({ session_id: id, events: buffer });
  buffer = [];

  try {
    if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
      const blob = new Blob([payload], { type: 'application/json' });
      if (navigator.sendBeacon(ENDPOINT, blob)) return;
    }
    void fetch(ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: payload,
      keepalive: true,
    }).catch(() => {
      // Analytics failing must never look like the site failing.
    });
  } catch {
    // Same.
  }
}

/**
 * `visibilitychange → hidden` rather than `unload`.
 *
 * Mobile Safari frequently never fires unload — the tab is frozen instead — so
 * an unload-based flush loses most phone traffic. visibilitychange fires in both
 * cases, and phones are how the factory's own staff will read this site.
 */
function bindListeners(): void {
  if (listenersBound || typeof document === 'undefined') return;
  listenersBound = true;
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flush();
  });
  window.addEventListener('pagehide', flush);
}

/** Test seam. Not used by the application. */
export function __resetForTests(): void {
  buffer = [];
  if (timer !== null) clearTimeout(timer);
  timer = null;
  listenersBound = false;
}
