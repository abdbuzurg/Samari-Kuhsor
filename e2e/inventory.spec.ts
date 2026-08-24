import { test, expect } from './fixtures';

/**
 * Склад — the ledger, and the constraint the whole module rests on.
 *
 * ToR §8 condition 4: warehouse balances by item, batch and expiry.
 */

test('every balance can be explained by opening its ledger', async ({ signedIn: page }) => {
  await page.goto('/inventory');
  await expect(page.getByTestId('list-row').first()).toBeVisible();
  await page.getByTestId('list-row').first().getByRole('link').first().click();

  // There is no stored balance behind the figure on the register: the rows here
  // are what it is made of (CLAUDE.md §4.2).
  await expect(page.getByTestId('detail-view')).toBeVisible();
  await expect(page.getByText('Движения')).toBeVisible();
  await expect(page.getByText('Остаток после')).toBeVisible();
});

test('NO form in the warehouse offers an absolute quantity', async ({ signedIn: page }) => {
  await page.goto('/inventory');
  await page.getByTestId('list-row').first().getByRole('link').first().click();

  await page.getByTestId('toggle-movement').click();
  await expect(page.getByText(/изменение остатка, а не итоговое количество/i)).toBeVisible();

  // 05-MODULES.md:112. A "set stock to X" control can only be implemented as an
  // update, which would make the append-only ledger a lie.
  const labels = await page.locator('#sk-content [aria-label]').evaluateAll((els) =>
    els.map((e) => e.getAttribute('aria-label') ?? ''),
  );
  expect(labels.some((l) => /остаток|итого|установить/i.test(l))).toBe(false);
});

test('an issue larger than the balance is refused with the server’s own message', async ({
  signedIn: page,
}) => {
  await page.goto('/inventory');
  await page.getByTestId('list-row').first().getByRole('link').first().click();

  await page.getByTestId('toggle-movement').click();
  await page.getByLabel('Причина').selectOption('sale');
  await page.getByLabel('Количество').fill('99999999');
  await page.getByTestId('save-movement').click();

  await expect(page.getByTestId('movement-error')).toBeVisible();
});

test('a transfer is offered only to a different location', async ({ signedIn: page }) => {
  await page.goto('/inventory');
  await page.getByTestId('list-row').first().getByRole('link').first().click();

  await page.getByTestId('toggle-transfer').click();
  const target = page.getByLabel('Куда');
  await expect(target).toBeVisible();
  // A transfer to itself is not a move.
  const options = await target.locator('option').count();
  expect(options).toBeGreaterThan(1);
});
