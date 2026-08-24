#!/usr/bin/env node
/**
 * Asserts the production compose file sets the env vars the API actually reads.
 *
 * `docker-compose.prod.yml` set DATABASE_URL and ADDR; `cmd/api/main.go` reads
 * DB_URL and LISTEN_ADDR. The API exited with "DB_URL is required" and the
 * container never became healthy, so nothing downstream started. It shipped
 * because the existing topology gate RENDERS this file and never boots it — the
 * artefacts were "verified locally" in the sense of being parseable.
 *
 * A name mismatch between a compose file and the binary it configures is
 * invisible to every test that does not start the process. This makes it
 * visible without starting one.
 *
 *   node tools/check-env-contract.mjs
 */
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import process from 'node:process';

/**
 * Vars the API requires to boot. MIGRATE_ON_START and PUBLIC_SITE_URL have
 * defaults in the binary, so they are not required here — but DB_URL has no
 * default and is a hard exit.
 */
const REQUIRED = ['DB_URL', 'SERVICE_KEY'];

/** Names the binary would silently ignore, with what it reads instead. */
const KNOWN_WRONG = {
  DATABASE_URL: 'DB_URL',
  ADDR: 'LISTEN_ADDR',
  PORT: 'LISTEN_ADDR',
  POSTGRES_URL: 'DB_URL',
};

function readsFromSource() {
  const src = readFileSync('backend/cmd/api/main.go', 'utf8');
  const names = new Set();
  for (const m of src.matchAll(/os\.Getenv\("([A-Z_]+)"\)|envOr\("([A-Z_]+)"/g)) {
    names.add(m[1] ?? m[2]);
  }
  return names;
}

function apiEnv() {
  const env = {
    ...process.env,
    POSTGRES_USER: 'check',
    POSTGRES_PASSWORD: 'check',
    SERVICE_KEY: 'check',
    PUBLIC_SITE_URL: 'http://203.0.113.10',
  };
  const raw = execFileSync(
    'docker',
    ['compose', '-f', 'docker-compose.prod.yml', 'config', '--format', 'json'],
    { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] },
  );
  return JSON.parse(raw).services.api.environment ?? {};
}

let set;
try {
  set = apiEnv();
} catch (err) {
  console.error('check-env-contract: could not render the production compose file.');
  console.error(String(err.stderr ?? err.message).trim());
  process.exit(1);
}

const reads = readsFromSource();
const problems = [];

for (const name of REQUIRED) {
  if (!(name in set)) {
    problems.push(`api is missing ${name}, which cmd/api/main.go requires to boot`);
  }
}

for (const [wrong, right] of Object.entries(KNOWN_WRONG)) {
  if (wrong in set) {
    problems.push(`api sets ${wrong}, which the binary ignores — it reads ${right}`);
  }
}

// Anything set but never read is either a typo or dead config. Both are worth
// knowing about; neither is fatal on its own.
const unread = Object.keys(set).filter((k) => !reads.has(k));
if (unread.length > 0) {
  console.warn(`check-env-contract: api sets vars the binary never reads: ${unread.join(', ')}`);
}

if (problems.length > 0) {
  console.error('check-env-contract: FAILED');
  for (const p of problems) console.error(`  - ${p}`);
  process.exit(1);
}

console.log(`check-env-contract: api env matches what cmd/api/main.go reads (${REQUIRED.join(', ')})`);
