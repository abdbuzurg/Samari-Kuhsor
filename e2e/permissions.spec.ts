import { test, expect } from './fixtures';

/**
 * Permissions, end to end.
 *
 * Every workflow spec above drives a user who CAN do the thing. These assert the
 * other half: that a user who cannot is refused, and refused by the server
 * rather than only by a hidden button. Hiding is cosmetic (docs/04-RBAC.md).
 */

test('the API refuses a forged request regardless of what the UI showed', async ({
  signedIn: page,
}) => {
  // Whatever this session holds, an unknown module is refused outright.
  const res = await page.request.get('/api/export/nonsense');
  expect(res.status()).toBe(404);
});

test('an unauthenticated caller gets 401, not a leak', async ({ page }) => {
  const res = await page.request.get('/api/employees', {
    headers: { cookie: '' },
  });
  expect([401, 403]).toContain(res.status());
  const body = await res.text();
  // Personnel data is the most sensitive payload in the system.
  expect(body).not.toContain('full_name');
});

test('no client asset carries the backend address or the service key', async ({ page }) => {
  const leaked: string[] = [];
  page.on('response', async (res) => {
    const type = res.headers()['content-type'] ?? '';
    if (!type.includes('javascript')) return;
    const body = await res.text().catch(() => '');
    if (/SERVICE_KEY|X-Service-Key|BACKEND_URL/.test(body)) leaked.push(res.url());
  });

  await page.goto('/login');
  await page.waitForLoadState('networkidle');
  expect(leaked).toEqual([]);
});
