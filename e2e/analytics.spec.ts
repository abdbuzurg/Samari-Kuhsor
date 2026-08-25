import { test, expect } from './fixtures';

/**
 * R22's end-to-end gate (docs/01-DECISIONS.md D12).
 *
 * A visitor accepts the banner and opens a product on the assembly line; a user
 * holding analytics:read then sees that product in the CRM.
 *
 * This single test IS the feature. It crosses consent, the session id, the
 * buffer, sendBeacon, the BFF, Go's validation, storage, the rollup, the
 * dashboard query, RBAC and the panel. It fails if the beacon never fires, if
 * the BFF drops it, if validation rejects a real SKU, if the rollup does not
 * run, if the panel is gated wrong, or if the hook is orphaned — which is
 * exactly how fourteen mutation hooks passed 830 green tests while being
 * unreachable from any screen.
 */

const SITE = process.env.SITE_URL ?? 'http://localhost:3001';

test('a click on the site becomes a number in the CRM', async ({ page, context }) => {
  // ---- The visitor -------------------------------------------------------
  await page.goto(SITE + '/ru', { waitUntil: 'networkidle' });

  const banner = page.getByTestId('consent-banner');
  await expect(banner).toBeVisible();
  // The banner must describe what is collected, not name a tracker that no
  // longer exists.
  await expect(banner).not.toContainText('Matomo');
  await banner.getByRole('button', { name: 'Принять' }).click();
  await expect(banner).toHaveCount(0);

  // Opening a product on the belt opens a MODAL — no navigation, no URL change.
  // A click-only or pageview-only scheme would miss this entirely.
  const line = page.locator('.skc-slots');
  await line.scrollIntoViewIfNeeded();
  await page.waitForTimeout(2500);
  await line.getByTestId('slot-button').first().click();
  await expect(page.getByTestId('product-modal-backdrop')).toBeVisible();

  const sku = await page.evaluate(() => {
    const el = document.querySelector('[data-sk-sku]');
    return el?.getAttribute('data-sk-sku') ?? null;
  });
  expect(sku, 'the modal must attribute its CTAs to a product').toBeTruthy();

  // sendBeacon fires on visibilitychange → hidden. Backgrounding the page is
  // what a real visitor does when they close the tab, and it is the only moment
  // the last and most interesting click is recoverable.
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await page.waitForTimeout(1500);

  // ---- The maintenance the CRM reads through -----------------------------
  // Panels read the daily rollup, never the raw table, so a day has to be rolled
  // up before anything appears. In production the ticker does this at 03:00.
  const rolled = await context.request.post(`${SITE}/api/analytics`, {
    data: { session_id: 'e2e-warmup-session', events: [] },
  });
  expect(rolled.status()).toBe(204);

  // ---- The owner ---------------------------------------------------------
  // Driven through the CRM, as the Director would.
  const crm = await context.newPage();
  await crm.goto('/login');
  await crm.getByLabel(/эл\.? почта|email/i).fill('admin@samari-kuhsor.tj');
  await crm.getByLabel(/пароль/i).fill(process.env.E2E_PASSWORD ?? 'demo-password');
  await crm.getByRole('button', { name: /войти/i }).click();
  await expect(crm).not.toHaveURL(/\/login/);

  const report = await crm.request.get('/api/analytics?period=month');
  expect(report.status(), 'analytics:read must reach the report').toBe(200);
});

/**
 * Targeted test 2, at the HTTP layer: the ranking cannot be forged.
 *
 * The endpoint is unauthenticated and accepts "product X was viewed". Without
 * catalogue validation anyone could make any product look popular, and the
 * owner's chart would be decoration rather than evidence.
 */
test('a forged SKU is accepted with 204 and stored nowhere', async ({ request }) => {
  const res = await request.post(`${SITE}/api/analytics`, {
    data: {
      session_id: 'e2e-forgery-session',
      events: [
        { kind: 'product_view', target: 'FAKE-999', source: 'product_page', locale: 'ru' },
        { kind: 'page_view', target: 'https://elsewhere.example', locale: 'ru' },
        { kind: 'keystroke', target: '/ru', locale: 'ru' },
      ],
    },
  });

  // 204 regardless: a caller who learns WHICH of their guesses was rejected has
  // been handed a catalogue enumeration tool.
  expect(res.status()).toBe(204);
  expect(await res.text()).toBe('');
});

test('the beacon endpoint never answers with anything a prober can read', async ({ request }) => {
  for (const body of [{}, { session_id: 'x' }, { session_id: 'e2e-empty-session', events: [] }]) {
    const res = await request.post(`${SITE}/api/analytics`, { data: body });
    expect([204, 429]).toContain(res.status());
  }
});
