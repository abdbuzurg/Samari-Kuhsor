import { describe, it, expect, vi, afterEach } from 'vitest';

/**
 * The robots.txt guard.
 *
 * It decides whether a deployment may be indexed. Getting it wrong in the
 * permissive direction puts a temporary address into search results for months;
 * getting it wrong the other way is a launch-day fix that takes minutes. The
 * asymmetry is why it fails closed, and why this is tested rather than assumed.
 */

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
});

async function robotsFor(url: string) {
  vi.stubEnv('PUBLIC_SITE_URL', url);
  vi.resetModules();
  const mod = await import('@/app/robots');
  return mod.default();
}

const PROVISIONAL = [
  'http://203.0.113.10',
  'http://203.0.113.10:8080',
  'http://127.0.0.1:3001',
  'http://localhost:3001',
  'https://localhost',
  'http://samari.local',
  'http://staging.internal',
  'http://qoim.test',
  // A single-label name resolves only inside somebody's network.
  'http://samari',
  // Unparseable: fails closed rather than throwing or allowing.
  'not-a-url',
  '',
];

describe('provisional addresses', () => {
  it.each(PROVISIONAL)('disallows everything on %s', async (url) => {
    const result = await robotsFor(url);
    const rules = Array.isArray(result.rules) ? result.rules : [result.rules];
    expect(rules[0].disallow).toBe('/');
    // No sitemap either: pointing crawlers at a temporary address is the same
    // mistake by another route.
    expect(result.sitemap).toBeUndefined();
  });
});

describe('the real domain', () => {
  it('allows crawling and points at the sitemap', async () => {
    const result = await robotsFor('https://samari-kuhsor.tj');
    const rules = Array.isArray(result.rules) ? result.rules : [result.rules];
    expect(rules[0].allow).toBe('/');
    expect(result.sitemap).toBe('https://samari-kuhsor.tj/sitemap.xml');
  });

  it('keeps the BFF out of the index', async () => {
    const result = await robotsFor('https://samari-kuhsor.tj');
    const rules = Array.isArray(result.rules) ? result.rules : [result.rules];
    // A crawler POSTing to the enquiry endpoint would burn the per-IP rate limit
    // that protects real visitors.
    expect(rules[0].disallow).toBe('/api/');
  });

  it('accepts a subdomain, which the CRM will be on', async () => {
    const result = await robotsFor('https://www.samari-kuhsor.tj');
    const rules = Array.isArray(result.rules) ? result.rules : [result.rules];
    expect(rules[0].allow).toBe('/');
  });

  it('tolerates a trailing slash without doubling it in the sitemap URL', async () => {
    const result = await robotsFor('https://samari-kuhsor.tj/');
    expect(result.sitemap).toBe('https://samari-kuhsor.tj/sitemap.xml');
  });
});
