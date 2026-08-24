import { test, expect, givenReleasedBatch } from './fixtures';

/**
 * ToR §5 workflow 4 — Complaint:
 *   complaint → customer/product identification → batch traceability →
 *   investigation → corrective action → closure.
 *
 * The CAPA register is phase 2 (docs/10-CLIENT-NOTICE.md §3). What is delivered
 * — and what this proves — is that a complaint arriving from the website opens
 * directly onto the batch it names, which is the step the whole traceability
 * claim rests on.
 */

test('a complaint opens onto the batch it names', async ({ signedIn: page }) => {
  await page.goto('/inquiries');
  const complaint = page.getByTestId('list-row').filter({ hasText: 'Жалоба' }).first();
  await expect(complaint).toBeVisible();
  await complaint.getByRole('link').first().click();

  await expect(page.getByText('Прослеживаемость')).toBeVisible();
  await page.getByRole('link', { name: /^B-/ }).click();

  // Landing on the batch's traceability view: tests, decision history, and where
  // the stock is now.
  await expect(page.getByTestId('detail-view')).toBeVisible();
  await expect(page.getByText('Лабораторные проверки')).toBeVisible();
  await expect(page.getByText('История решений')).toBeVisible();
  await expect(page.getByText('Где находится')).toBeVisible();
});

test('a released batch can be recalled, and the recall demands a reason', async ({
  signedIn: page,
}) => {
  const batch = await givenReleasedBatch(page);
  await page.goto(`/quality/${batch.id}`);
  await expect(page.getByTestId('detail-view')).toBeVisible();

  await page.getByRole('button', { name: 'Забраковать' }).click();
  // The domain refuses a recall without a stated cause, so the UI collects one
  // before sending rather than showing a refusal.
  await expect(page.getByTestId('transition-reason')).toBeVisible();
  await expect(page.getByTestId('confirm-transition')).toBeDisabled();

  await page.getByRole('textbox').last().fill('Жалоба потребителя: посторонний привкус');
  await page.getByTestId('confirm-transition').click();

  await expect(page.getByTestId('related-row').filter({ hasText: 'посторонний привкус' })).toBeVisible();
});
