#!/usr/bin/env node
/**
 * Asserts the production topology's security model.
 *
 * CLAUDE.md §3 says the browser never calls the Go API directly. That is only
 * true if the api container has no host port — a convention people follow is not
 * the same as a fact about the network, and this is what makes it the latter.
 *
 * Run against the RENDERED compose config, not the source YAML: an anchor, an
 * override file or a `ports` line added under a profile would all escape a
 * regex over the file, and the rendered config is what Docker actually uses.
 *
 *   node tools/check-topology.mjs
 */
import { execFileSync } from 'node:child_process';
import process from 'node:process';

const ALLOWED_TO_PUBLISH = new Set(['caddy']);

function render() {
  // Placeholder values: this checks topology, not secrets.
  const env = {
    ...process.env,
    POSTGRES_USER: 'check',
    POSTGRES_PASSWORD: 'check',
    SERVICE_KEY: 'check',
    PUBLIC_SITE_URL: 'http://203.0.113.10',
  };
  return execFileSync(
    'docker',
    ['compose', '-f', 'docker-compose.prod.yml', 'config', '--format', 'json'],
    { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] },
  );
}

let config;
try {
  config = JSON.parse(render());
} catch (err) {
  console.error('check-topology: could not render the production compose file.');
  console.error(String(err.stderr ?? err.message ?? err).split('\n').slice(0, 4).join('\n'));
  process.exit(1);
}

const failures = [];
for (const [name, service] of Object.entries(config.services ?? {})) {
  const ports = service.ports ?? [];
  if (ports.length > 0 && !ALLOWED_TO_PUBLISH.has(name)) {
    failures.push(
      `${name} publishes ${ports.map((p) => p.published ?? p.target).join(', ')} to the host. ` +
        `Only caddy may. See the ports comment in docker-compose.prod.yml.`,
    );
  }
}

// The api must additionally never be reachable, even from localhost. Checked
// separately from the loop so the message names the actual rule it breaks.
const api = config.services?.api;
if (api && (api.ports ?? []).length > 0) {
  failures.push(
    'api publishes a host port. The whole "browser never calls Go directly" ' +
      'guarantee (CLAUDE.md §3) rests on it having none.',
  );
}

if (failures.length > 0) {
  console.error('check-topology: FAILED');
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}

const published = Object.entries(config.services ?? {})
  .filter(([, s]) => (s.ports ?? []).length > 0)
  .map(([n]) => n);
console.log(
  `check-topology: ${Object.keys(config.services ?? {}).length} services, ` +
    `only ${published.join(', ') || 'none'} published to the host`,
);
