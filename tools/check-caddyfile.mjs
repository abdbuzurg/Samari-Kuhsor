#!/usr/bin/env node
/**
 * Validates deploy/Caddyfile in every TLS stage, with the env vars empty.
 *
 * The Caddyfile had `email {$ACME_EMAIL}` and a comment asserting that an empty
 * value was fine. It is not: an empty variable renders as a bare `email` with no
 * argument, Caddy refuses the entire config, and the container crash-loops —
 * taking the public site and the CRM down with it. The API, database, CRM and
 * web containers were all healthy at the time, which made it look like a
 * networking problem rather than a one-line syntax error.
 *
 * It shipped for the same reason the compose env mismatch did: these artefacts
 * were written and reviewed, never executed. `caddy validate` costs two seconds.
 *
 * Empty strings rather than unset vars, because that is what docker compose
 * actually passes: `ACME_EMAIL: ${ACME_EMAIL:-}` sets the variable to "".
 *
 *   node tools/check-caddyfile.mjs
 */
import { execFileSync } from 'node:child_process';
import process from 'node:process';

const CASES = [
  { name: 'TLS_MODE=off, nothing set (stage 1)', env: { TLS_MODE: 'off', ACME_EMAIL: '', SITE_HOST: ':80', CRM_HOST: ':80' } },
  { name: 'TLS_MODE=internal (staging rehearsal)', env: { TLS_MODE: 'internal', ACME_EMAIL: '', SITE_HOST: ':80', CRM_HOST: ':80' } },
  { name: 'TLS_MODE=auto with hostnames (T38)', env: { TLS_MODE: 'auto', ACME_EMAIL: 'ops@samari-kuhsor.tj', SITE_HOST: 'samari-kuhsor.tj', CRM_HOST: 'crm.samari-kuhsor.tj' } },
];

let failed = false;

for (const { name, env } of CASES) {
  const args = ['run', '--rm'];
  for (const [k, v] of Object.entries(env)) args.push('-e', `${k}=${v}`);
  args.push(
    '-v', `${process.cwd()}/deploy/Caddyfile:/etc/caddy/Caddyfile:ro`,
    'caddy:2-alpine',
    'caddy', 'validate', '--config', '/etc/caddy/Caddyfile',
  );

  try {
    execFileSync('docker', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
    console.log(`check-caddyfile: ok — ${name}`);
  } catch (err) {
    failed = true;
    const out = String(err.stderr ?? err.stdout ?? err.message);
    const line = out.split('\n').find((l) => l.startsWith('Error:')) ?? out.trim();
    console.error(`check-caddyfile: FAILED — ${name}`);
    console.error(`  ${line}`);
  }
}

process.exit(failed ? 1 : 0);
