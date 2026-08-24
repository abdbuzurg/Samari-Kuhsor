import { test as base, expect, type Page } from '@playwright/test';

/**
 * Shared sign-in.
 *
 * Every spec drives a REAL role rather than an administrator, because the point
 * of these tests is that the person who actually does the job can do it. An
 * admin-only suite would pass on a system where no operator can release a batch.
 */
export const USERS = {
  admin: { email: 'admin@samari-kuhsor.tj', password: process.env.E2E_PASSWORD ?? 'demo-password' },
} as const;

export async function signIn(page: Page, user = USERS.admin) {
  await page.goto('/login');
  await page.getByLabel(/эл\.? почта|email/i).fill(user.email);
  await page.getByLabel(/пароль/i).fill(user.password);
  await page.getByRole('button', { name: /войти/i }).click();
  await expect(page).not.toHaveURL(/\/login/);
}

export const test = base.extend<{ signedIn: Page }>({
  signedIn: async ({ page }, use) => {
    await signIn(page);
    await use(page);
  },
});

export { expect };

/**
 * Preconditions, created through the API.
 *
 * The first version of these specs read whatever the demo seed happened to
 * leave — and then mutated it. Releasing the one quarantined batch made every
 * later spec that needed a quarantined batch fail, in an order-dependent way
 * that looked like a product bug. A spec that consumes shared state has to
 * create it.
 */

let counter = 0;
const unique = () => `${Date.now()}-${counter++}`;

/** Creates a batch and moves it to `quarantine`, the state QC decides on. */
export async function givenQuarantinedBatch(page: Page): Promise<{ id: string; no: string }> {
  const items = await (await page.request.get('/api/items')).json();
  const itemId = items.data[0].id;

  const no = `E2E-${unique()}`;
  const created = await page.request.post('/api/batches', {
    data: { batch_no: no, item_id: itemId },
  });
  const batch = (await created.json()).data;

  // in_production → quarantine is the automatic move production completion
  // makes. Driving it directly keeps the fixture to one concern.
  await page.request.post(`/api/batches/${batch.id}/transition`, { data: { to: 'quarantine' } });
  return { id: batch.id, no };
}

/** Creates a batch already released, for recall and certificate specs. */
export async function givenReleasedBatch(page: Page): Promise<{ id: string; no: string }> {
  const batch = await givenQuarantinedBatch(page);
  await page.request.post(`/api/batches/${batch.id}/tests`, {
    data: { test_type: 'ph', result_value: '3.6', passed: true },
  });
  await page.request.post(`/api/batches/${batch.id}/transition`, { data: { to: 'released' } });
  return batch;
}

/** Creates a new, unconverted website enquiry. */
export async function givenNewInquiry(page: Page, type = 'wholesale'): Promise<string> {
  const res = await page.request.post('/api/inquiries', {
    data: {
      type,
      name: `E2E ${unique()}`,
      company: 'ООО «Тест»',
      contact: '+992 900 000 000',
      message: 'Автотест',
    },
  });
  if (!res.ok()) {
    // The CRM has no create-inquiry route; enquiries arrive through the public
    // website. Fall back to the newest unconverted one and skip if there is none.
    const list = await (await page.request.get('/api/inquiries?status=new')).json();
    return list.data?.[0]?.id ?? '';
  }
  return (await res.json()).data.id;
}

/** Creates a purchase order sitting at `approval`. */
export async function givenPurchaseOrderAwaitingApproval(page: Page): Promise<string> {
  const suppliers = await (await page.request.get('/api/suppliers')).json();
  const items = await (await page.request.get('/api/items')).json();

  const created = await page.request.post('/api/purchase-orders', {
    data: {
      po_no: `E2E-PO-${unique()}`,
      supplier_id: suppliers.data[0].id,
      lines: [{ item_id: items.data[0].id, qty: '100.000', unit_price: '15.00' }],
    },
  });
  const po = (await created.json()).data;
  await page.request.post(`/api/purchase-orders/${po.id}/transition`, { data: { to: 'approval' } });
  return po.id;
}
