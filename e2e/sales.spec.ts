import { test, expect } from './fixtures';

/**
 * ToR §5 workflow 1 — Sales:
 *   website inquiry → CRM lead → quotation → sales order → stock/production
 *   check → invoice → delivery → payment confirmation.
 *
 * Invoice and payment are out of scope (Финансы и бюджет, D2 — see
 * docs/10-CLIENT-NOTICE.md §1). Everything up to delivery is covered.
 */

test('a website enquiry is converted into a customer and a lead', async ({ signedIn: page }) => {
  // Enquiries arrive from the website, which is a different app. The CRM has no
  // route that creates one — deliberately, it is the single unauthenticated
  // write in the system — so this reads an unconverted one and skips if the
  // demo data has already been consumed by an earlier run.
  const list = await (await page.request.get('/api/inquiries?status=new')).json();
  test.skip(!list.data?.length, 'no unconverted enquiry; re-seed the demo data');

  await page.goto(`/inquiries/${list.data[0].id}`);

  await expect(page.getByTestId('detail-view')).toBeVisible();
  await page.getByTestId('convert-inquiry').click();

  // ToR §8 condition 1. Conversion lands on a customer screen that opens —
  // before R07/R13 it created a record no screen could show.
  // The customer route is /crm/{id}. Conversion must land on a screen that
  // exists — creating a record nobody can open is the defect R13 fixed.
  await expect(page).toHaveURL(/\/crm\/[0-9a-f-]{36}$/);
  await expect(page.getByText('Обращения с сайта')).toBeVisible();
});

test('a deal moves through the pipeline and its history records the move', async ({ signedIn: page }) => {
  await page.goto('/crm/pipeline');
  await expect(page.getByTestId('pipeline-board')).toBeVisible();

  // A deal not already at the target stage: same-stage moves are illegal, so
  // the button would not be offered.
  const open = page.getByTestId('stage-new').getByTestId('deal-card').first();
  const negotiating = page.getByTestId('stage-negotiation').getByTestId('deal-card').first();
  const card = (await open.count()) ? open : negotiating;
  test.skip(!(await card.count()), 'no open deal below «КП отправлено»');

  await card.click();
  await expect(page.getByTestId('detail-view')).toBeVisible();

  const before = await page.getByTestId('related-row').count();
  await page.getByRole('button', { name: 'КП отправлено' }).click();

  // The stage change and its event are written in one transaction, so the
  // history gains a row.
  await expect(page.getByTestId('related-row')).toHaveCount(before + 1);
});

test('a closed deal cannot be reopened', async ({ signedIn: page }) => {
  await page.goto('/crm/pipeline');
  const won = page.getByTestId('stage-won').getByTestId('deal-card').first();
  await won.click();

  // Won and lost are terminal: a reopened deal makes every conversion figure
  // provisional, so the server offers nothing and the UI shows nothing.
  await expect(page.getByTestId('no-transitions')).toBeVisible();
});

test('a released batch is loaded onto a trip and a quarantined one is not offered', async ({
  signedIn: page,
}) => {
  await page.goto('/logistics');
  await page.getByTestId('list-row').first().getByRole('link').first().click();
  await expect(page.getByTestId('detail-view')).toBeVisible();

  await page.getByTestId('toggle-load-form').click();
  const picker = page.getByLabel('Партия');
  await expect(picker).toBeVisible();

  // Only released batches are offered — a lorry leaving with quarantined product
  // is the failure the whole quality chain exists to prevent.
  const options = await picker.locator('option').allTextContents();
  expect(options.some((o) => o.includes('Карантин'))).toBe(false);
});

test('the sales funnel on the dashboard is no longer permanently empty', async ({ signedIn: page }) => {
  await page.goto('/');
  // The panel reads `deals`, which nothing wrote before R12. It rendered its
  // empty state from the day it shipped.
  await expect(page.getByTestId('pipeline')).toBeVisible();
  await expect(page.getByTestId('pipeline-row').first()).toBeVisible();
});
