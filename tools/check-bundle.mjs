// Verifies that no server-side secret reached a client bundle.
//
// CLAUDE.md §3 is a non-negotiable boundary: "No backend URL, token or service
// credential may appear in client-side code." That is exactly the kind of rule
// that holds for months and then quietly breaks the first time someone imports a
// server helper into a component — so it is checked mechanically, on the built
// output, not reviewed by eye.
//
//   node tools/check-bundle.mjs apps/crm [apps/web]

import fs from 'node:fs';
import path from 'node:path';

const FORBIDDEN = [
  { pattern: /SERVICE_KEY/, why: 'the BFF service credential' },
  { pattern: /X-Service-Key/i, why: 'the service-key header the BFF sends' },
  { pattern: /BACKEND_URL/, why: 'the Go API address' },
  { pattern: /\/api\/v1\//, why: "the Go API's own path prefix (the browser must call /api/* on this origin)" },
];

function* files(dir) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* files(full);
    else if (/\.(js|mjs|css)$/.test(entry.name)) yield full;
  }
}

let failures = 0;
let scanned = 0;

for (const app of process.argv.slice(2)) {
  // Only what is actually served to the browser.
  const clientDir = path.join(app, '.next', 'static');
  if (!fs.existsSync(clientDir)) {
    console.error(`check-bundle: ${clientDir} not found — run the build first`);
    process.exit(1);
  }

  for (const file of files(clientDir)) {
    scanned++;
    const source = fs.readFileSync(file, 'utf8');
    for (const { pattern, why } of FORBIDDEN) {
      if (pattern.test(source)) {
        console.error(`check-bundle: ${file}\n  contains ${pattern} — ${why}`);
        failures++;
      }
    }
  }
}

if (failures) {
  console.error(`\ncheck-bundle: ${failures} violation(s) of the CLAUDE.md §3 boundary`);
  process.exit(1);
}
console.log(`check-bundle: ${scanned} client assets scanned, no server secrets present`);
