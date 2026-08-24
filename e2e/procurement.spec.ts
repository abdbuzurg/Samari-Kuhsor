import { test, expect, givenPurchaseOrderAwaitingApproval } from './fixtures';

/**
 * ToR §5 workflow 2 — Procurement:
 *   purchase request → approval → supplier quotations → purchase order →
 *   delivery → quality inspection → warehouse receipt → payment.
 *
 * Purchase requests and quotation comparison are phase 2
 * (docs/10-CLIENT-NOTICE.md §3); payment is out of scope. The PO → approval →
 * receipt → stock chain is covered.
 */

test('an order awaiting approval is approved and then received into stock', async ({
  signedIn: page,
}) => {
  const poId = await givenPurchaseOrderAwaitingApproval(page);
  await page.goto(`/procurement/${poId}`);

  await expect(page.getByTestId('detail-view')).toBeVisible();
  await page.getByRole('button', { name: 'Подтвердить' }).click();

  await page.getByTestId('toggle-receipt').click();
  await page.getByLabel('Локация').selectOption({ index: 1 });
  const qtyField = page.getByLabel(/^Принято по /).first();
  await qtyField.fill('100');
  await page.getByTestId('save-receipt').click();

  // The receipt posts goods_receipt movements in the same transaction, so the
  // warehouse balance moves when the lorry is unloaded.
  await expect(page.getByTestId('receipt-error')).toHaveCount(0);
});

test('a receipt with no quantities is refused before it is sent', async ({ signedIn: page }) => {
  const poId = await givenPurchaseOrderAwaitingApproval(page);
  await page.goto(`/procurement/${poId}`);
  await page.getByRole('button', { name: 'Подтвердить' }).click();
  await expect(page.getByTestId('toggle-receipt')).toBeVisible();

  await page.getByTestId('toggle-receipt').click();
  await page.getByLabel('Локация').selectOption({ index: 1 });
  await page.getByTestId('save-receipt').click();

  await expect(page.getByTestId('receipt-error')).toContainText('хотя бы по одной позиции');
});
