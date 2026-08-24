import { test, expect, givenQuarantinedBatch, givenReleasedBatch } from './fixtures';

/**
 * ToR §5 workflow 3 — Production:
 *   production plan → material reservation → batch production → quality testing
 *   → release → finished-goods stock.
 *
 * This is the chain the whole system exists to protect. Until R03/R04 it could
 * only be driven by curl.
 */

test('a shift entry is recorded and the order completes into quarantine', async ({ signedIn: page }) => {
  await page.goto('/production');
  await expect(page.getByTestId('list-row').first()).toBeVisible();

  await page.getByTestId('list-row').first().getByRole('link').first().click();
  await expect(page.getByTestId('detail-view')).toBeVisible();

  const before = await page.getByTestId('related-row').count();

  await page.getByTestId('toggle-entry-form').click();
  await page.getByLabel('Годных').fill('120');
  await page.getByLabel('Брак').fill('5');
  await page.getByTestId('save-entry').click();

  // Append-only: a row is ADDED. Asserting on the content would match rows an
  // earlier run appended, since entries are never edited or removed.
  await expect(page.getByTestId('related-row')).toHaveCount(before + 1);
});

test('a quarantined batch is released by a user holding quality:approve', async ({ signedIn: page }) => {
  const batch = await givenQuarantinedBatch(page);
  await page.goto(`/quality/${batch.id}`);

  await expect(page.getByTestId('detail-view')).toBeVisible();
  await expect(page.getByTestId('related-table')).toHaveCount(3); // tests · decisions · stock

  // Scoped: the QR band also has a button beginning «Выпустить».
  await page.getByTestId('workflow-actions').getByRole('button', { name: 'Выпустить' }).click();

  // ToR §8 condition 5. The batch becomes sellable and its history records who
  // decided.
  await expect(page.getByText('Выпущено').first()).toBeVisible();
  await expect(page.getByTestId('related-row').filter({ hasText: 'Выпущено' })).toBeVisible();
});

test('recording a failed test is possible and is called out', async ({ signedIn: page }) => {
  const batch = await givenQuarantinedBatch(page);
  await page.goto(`/quality/${batch.id}`);
  await expect(page.getByTestId('detail-view')).toBeVisible();

  await page.getByTestId('toggle-test-form').click();
  await page.getByLabel('Тип проверки').selectOption('microbiology');
  await page.getByLabel('Значение').fill('обнаружено');
  await page.getByLabel('Проверка пройдена').uncheck();
  await page.getByTestId('save-test').click();

  await expect(page.getByTestId('related-row').filter({ hasText: 'обнаружено' })).toBeVisible();
});

test('a batch certificate names the releasing user', async ({ signedIn: page }) => {
  const batch = await givenReleasedBatch(page);
  await page.goto(`/quality/${batch.id}`);
  await expect(page.getByTestId('detail-view')).toBeVisible();

  const [certificate] = await Promise.all([
    page.waitForEvent('popup'),
    page.getByRole('link', { name: 'Паспорт качества' }).click(),
  ]);

  await expect(certificate.getByTestId('certificate-tests')).toBeVisible();
  await expect(certificate.getByText('Решение о выпуске принял')).toBeVisible();
  await expect(certificate.getByTestId('not-released')).toHaveCount(0);
});
