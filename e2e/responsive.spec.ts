import { test, expect } from './fixtures';

/**
 * I27 — the responsive pass, at the phone the factory floor actually uses.
 *
 * Runs only under the `mobile` project (390px). The sidebar is a fixed 252px at
 * desktop by contract (CLAUDE.md §5); below `lg` it becomes a drawer, because a
 * fixed sidebar on a 390px screen leaves 138px of content.
 */

test('the sidebar is a drawer, and the page never scrolls sideways', async ({ signedIn: page }) => {
  await page.goto('/quality');

  await expect(page.getByTestId('open-nav')).toBeVisible();
  await page.getByTestId('open-nav').click();
  await expect(page.getByTestId('sidebar')).toHaveAttribute('data-open', 'true');

  await page.getByTestId('nav-scrim').click();
  await expect(page.getByTestId('sidebar')).toHaveAttribute('data-open', 'false');

  // Wide tables scroll inside their own container; the body must not.
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
  expect(overflow).toBe(false);
});

test('a detail view stacks rather than squeezing the activity panel', async ({ signedIn: page }) => {
  await page.goto('/quality');
  await page.getByTestId('list-row').first().getByRole('link').first().click();
  await expect(page.getByTestId('detail-view')).toBeVisible();

  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
  expect(overflow).toBe(false);
});
