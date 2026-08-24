import { defineConfig, devices } from '@playwright/test';

/**
 * R18 — end-to-end specs for the five ToR §5 workflows.
 *
 * T34's first full run was driven by curl. That proved the chain works over
 * HTTP; it proved nothing about whether a human can drive it, which is exactly
 * the gap the R-plan exists to close. These specs drive the browser.
 *
 * They assume a stack is already running — Postgres, the Go API and both Next
 * apps — seeded with `seed reference` and `seed demo`. `make e2e` brings that up.
 * Playwright does not start it, because the stack is four processes and a
 * database and hiding that behind a test runner makes failures unreadable.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // the specs share one database
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? 'github' : 'list',
  timeout: 30_000,
  use: {
    baseURL: process.env.CRM_URL ?? 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    locale: 'ru-RU',
    timezoneId: 'Asia/Dushanbe',
  },
  projects: [
    {
      name: 'desktop',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
      // The responsive spec asserts the drawer, which does not exist above lg.
      testIgnore: /responsive\.spec\.ts/,
    },
    // I27: the responsive pass. 390px is the phone the factory floor actually uses.
    { name: 'mobile', use: { ...devices['Pixel 5'] }, testMatch: /responsive\.spec\.ts/ },
  ],
});
