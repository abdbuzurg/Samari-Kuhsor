import { test, expect } from '@playwright/test';

/**
 * The public website at phone width.
 *
 * The site was ported 1:1 from a desktop prototype whose own mobile CSS covered
 * exactly four things: the assembly line becoming a horizontal snap scroller,
 * the hero and two-column grids collapsing, and the marquee speeding up.
 * Everything else was authored at desktop width, and the header alone forced
 * 673px of horizontal scroll on every page.
 *
 * These run under the `mobile` project (Pixel 5, 393px). The website is a
 * different app from the CRM, so it has its own base URL.
 */

const SITE = process.env.SITE_URL ?? 'http://localhost:3001';

const PAGES = [
  ['home', '/ru'],
  ['catalogue', '/ru/catalogue'],
  ['product', '/ru/catalogue/APJ-1000'],
  ['production', '/ru/production'],
  ['contact', '/ru/contact'],
  ['privacy', '/ru/privacy'],
] as const;

for (const [name, path] of PAGES) {
  test(`${name} does not scroll sideways`, async ({ page }) => {
    await page.goto(SITE + path, { waitUntil: 'networkidle' });
    // The whole point. Every one of these overflowed by 673px before the
    // header became a drawer.
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow, `${path} overflows by ${overflow}px`).toBeLessThanOrEqual(1);
  });
}

test('the header is a drawer, and it carries everything the desktop nav does', async ({ page }) => {
  await page.goto(SITE + '/ru', { waitUntil: 'networkidle' });

  const burger = page.getByTestId('site-burger');
  await expect(burger).toBeVisible();
  await expect(page.getByTestId('site-drawer')).toHaveCount(0);

  await burger.click();
  const drawer = page.getByTestId('site-drawer');
  await expect(drawer).toBeVisible();

  // Four nav items and ТҶ / РУ / EN in that order — the visual contract.
  await expect(drawer.locator('nav a')).toHaveCount(4);
  await expect(drawer.locator('[role="group"] a')).toHaveText(['ТҶ', 'РУ', 'EN']);
});

test('a drawer link navigates and does not stay open over the destination', async ({ page }) => {
  await page.goto(SITE + '/ru', { waitUntil: 'networkidle' });
  await page.getByTestId('site-burger').click();
  await page.getByTestId('site-drawer').locator('nav a').nth(1).click();

  await expect(page).toHaveURL(/\/ru\/catalogue$/);
  await expect(page.getByTestId('site-drawer')).toHaveCount(0);
});

/**
 * CLAUDE.md §5, verbatim: "The assembly line stays horizontal and swipeable on
 * mobile. It must never become a vertical list."
 *
 * This is the one mobile behaviour the client specified directly, so it gets a
 * test that would fail if a future responsive pass "helpfully" stacked it.
 */
test('the assembly line stays horizontal and swipes', async ({ page }) => {
  await page.goto(SITE + '/ru', { waitUntil: 'networkidle' });

  const line = page.locator('.skc-slots');
  await expect(line).toBeVisible();

  const shape = await line.evaluate((el) => {
    const cs = getComputedStyle(el);
    const tops = [...el.querySelectorAll('.skc-slot')].map((k) =>
      Math.round(k.getBoundingClientRect().top),
    );
    return {
      display: cs.display,
      overflowX: cs.overflowX,
      snap: cs.scrollSnapType,
      scrollable: el.scrollWidth > el.clientWidth,
      distinctRows: new Set(tops).size,
    };
  });

  expect(shape.display).toBe('flex');
  expect(shape.overflowX).toBe('auto');
  expect(shape.snap).toContain('x');
  expect(shape.scrollable).toBe(true);
  // One row. Stacked slots would report one distinct top per slot.
  expect(shape.distinctRows).toBe(1);

  const before = await line.evaluate((el) => el.scrollLeft);
  await line.evaluate((el) => el.scrollBy({ left: 300 }));
  await page.waitForTimeout(400);
  expect(await line.evaluate((el) => el.scrollLeft)).toBeGreaterThan(before);
});

/**
 * Target size, against the standard rather than a round number.
 *
 * WCAG 2.5.8 Target Size (Minimum) is AA and asks for 24×24 CSS px. The 44px
 * figure people quote is 2.5.5, which is AAA. The distinction matters here:
 * chasing 44 would mean padding out «Вся продукция →» until its accent
 * underline detached from the text, which is a client-approved detail
 * (CLAUDE.md §5) and not ours to change for a level nobody asked for.
 *
 * So: 24px is the gate. Controls that could be made larger for free — the
 * consent buttons, the batch pager, the catalogue filters — were, and are
 * comfortably above it.
 *
 * Inline links inside body copy are not selected at all: WCAG exempts them, and
 * padding them out breaks the line box of every link on the site.
 */
test('every standalone control meets the WCAG 2.5.8 target size', async ({ page }) => {
  await page.goto(SITE + '/ru', { waitUntil: 'networkidle' });

  const tooSmall = await page.evaluate(() =>
    [...document.querySelectorAll('button, a[class*="skc-tap"]')]
      .map((el) => ({ el, r: el.getBoundingClientRect() }))
      .filter(({ r }) => r.width > 0 && r.height > 0 && (r.height < 24 || r.width < 24))
      .map(
        ({ el, r }) =>
          `${el.tagName.toLowerCase()} ${Math.round(r.width)}×${Math.round(r.height)} "${(el.textContent ?? '').trim().slice(0, 24)}"`,
      ),
  );

  expect(tooSmall, `undersized controls: ${tooSmall.join(' | ')}`).toEqual([]);
});
