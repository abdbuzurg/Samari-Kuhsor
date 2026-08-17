// Extracts the approved website prototype into apps/web.
//
// design/Samari Kuhsor - website.html is a compiled single-file bundle: a manifest
// of base64 resources plus an escaped HTML template that a small React runtime
// parses at load time (docs/07-IMPLEMENTATION-PLAN.md C4).
//
// This script recovers the authored source and its assets so the port (T29) is a
// translation of real source rather than a rebuild from screenshots. It is
// idempotent and never writes into design/, which is read-only.
//
//   node tools/extract-website.mjs

import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const BUNDLE = path.join(ROOT, 'design', 'Samari Kuhsor - website.html');
const ASSETS = path.join(ROOT, 'apps', 'web', 'public', 'assets');
const REF = path.join(ROOT, 'apps', 'web', '.reference');

// Identified from how the template references each uuid — by alt text and by the
// style binding it carries. See the mapping note in .reference/ASSETS.md.
const ASSET_NAMES = {
  'e68ba3ff-3e52-4e44-be31-faaf2f62a6db': 'logo-samari-mark.png', // header + footer, alt "Самари Кӯҳсор"
  '979349be-5af8-4e07-bc91-c191ecb7373e': 'logo-qoim.png',        // About, Contacts, footer, alt "QOIM"
  '12a1f254-7d88-4c03-8a69-10ad3b604ef6': 'map-base.png',         // animated map, bound to mapFillStyle
  '855618b8-5025-4f96-915b-923312f05e32': 'map-heart.png',        // heart marker, bound to mapHeartStyle
  '3c0de88c-7425-4e4c-b6a9-aa15a314fa2f': 'map-full.jpg',         // static map on Contacts
};

function readBlock(html, type) {
  const m = html.match(new RegExp(`<script type="__bundler/${type}">\\s*([\\s\\S]*?)\\s*</script>`));
  if (!m) throw new Error(`bundle is missing its ${type} block`);
  return m[1];
}

function decode(entry) {
  const buf = Buffer.from(entry.data, 'base64');
  return entry.compressed ? zlib.gunzipSync(buf) : buf;
}

const html = fs.readFileSync(BUNDLE, 'utf8');
const manifest = JSON.parse(readBlock(html, 'manifest'));
const template = JSON.parse(readBlock(html, 'template'));

fs.mkdirSync(ASSETS, { recursive: true });
fs.mkdirSync(REF, { recursive: true });

// ---------------------------------------------------------------------------
// Assets
// ---------------------------------------------------------------------------
const written = [];
const fonts = [];

for (const [uuid, entry] of Object.entries(manifest)) {
  if (entry.mime.startsWith('image/')) {
    const name = ASSET_NAMES[uuid];
    if (!name) throw new Error(`unmapped image ${uuid} (${entry.mime}) — identify it before extracting`);
    const bytes = decode(entry);
    fs.writeFileSync(path.join(ASSETS, name), bytes);
    written.push({ name, bytes: bytes.length, uuid });
  } else if (entry.mime.startsWith('font/')) {
    fonts.push({ uuid, bytes: decode(entry).length });
  }
}

// ---------------------------------------------------------------------------
// Source: markup, styles, logic
// ---------------------------------------------------------------------------
fs.writeFileSync(path.join(REF, 'site-source.html'), template);

const styles = [...template.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]);
const scripts = [...template.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)]
  .map((m) => m[1])
  .filter((s) => s.trim().length > 0);

// Block 0 is the @font-face set injected by the bundler; the authored CSS follows.
// Keep them separate so the port ships the authored CSS verbatim (I19) and takes
// fonts from next/font instead.
fs.writeFileSync(path.join(REF, 'site-fontface.css'), styles[0] ?? '');
fs.writeFileSync(path.join(REF, 'site.css'), styles.slice(1).join('\n\n'));
fs.writeFileSync(path.join(REF, 'site-logic.js'), scripts.join('\n\n'));

// Which Golos Text faces the bundle actually carries. If these are weight 400 only,
// the extracted woff2 are insufficient for the design (400–900) and next/font is
// required rather than merely preferable.
// Scope to one @font-face block at a time; a regex spanning blocks silently
// pairs a family from one declaration with a weight from the next.
const faces = [...(styles[0] ?? '').matchAll(/@font-face\s*\{([^}]*)\}/g)].map((m) => {
  const body = m[1];
  const field = (name) => (body.match(new RegExp(`${name}:\\s*([^;]+);`)) || [, ''])[1].trim();
  const src = field('src');
  return {
    family: field('font-family').replace(/['"]/g, ''),
    weight: field('font-weight'),
    inlined: /^url\(["']?[0-9a-f-]{36}/.test(src), // a bundle uuid, not a remote URL
  };
});
const inlinedWeights = [...new Set(faces.filter((f) => f.inlined).map((f) => f.weight))].sort();
const declaredWeights = [...new Set(faces.map((f) => f.weight))].sort();

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------
const report = [
  '# Extracted from the approved website prototype',
  '',
  'Generated by `tools/extract-website.mjs`. Do not hand-edit — re-run the script.',
  'Source: `design/Samari Kuhsor - website.html` (read-only).',
  '',
  '## Assets → `apps/web/public/assets/`',
  '',
  '| File | Bytes | Bundle id |',
  '|---|---:|---|',
  ...written.sort((a, b) => a.name.localeCompare(b.name))
    .map((w) => `| \`${w.name}\` | ${w.bytes.toLocaleString('en-US')} | \`${w.uuid}\` |`),
  '',
  '## Source → `apps/web/.reference/`',
  '',
  '| File | Purpose |',
  '|---|---|',
  '| `site-source.html` | Authored markup with `sc-*` directives. Translate to JSX (T29). |',
  '| `site.css` | Authored CSS. Ships **verbatim** per I19. |',
  '| `site-logic.js` | Component state and content arrays. Content moves to CMS/`items`. |',
  '| `site-fontface.css` | Bundler-injected `@font-face`. Superseded by `next/font`. |',
  '',
  '## Golos Text faces',
  '',
  `- \`@font-face\` blocks: **${faces.length}**`,
  `- Weights declared: **${declaredWeights.join(', ') || 'none'}**`,
  `- Weights actually inlined in the bundle: **${inlinedWeights.join(', ') || 'none'}**`,
  `- woff2 files in the manifest: **${fonts.length}**`,
  '',
  inlinedWeights.length < declaredWeights.length
    ? '> The bundle declares more weights than it inlines — the rest resolve to remote Google\n' +
      '> Fonts URLs and would fail on a box with no outbound internet. The port takes Golos Text\n' +
      '> from `next/font/google`, which self-hosts every declared weight at build time.'
    : '> The port takes Golos Text from `next/font/google` regardless, so these files are reference only.',
  '',
].join('\n');

fs.writeFileSync(path.join(REF, 'ASSETS.md'), report);

console.log(`assets:  ${written.length} → apps/web/public/assets/`);
for (const w of written.sort((a, b) => a.name.localeCompare(b.name))) {
  console.log(`         ${w.name.padEnd(24)} ${String(w.bytes).padStart(8)} bytes`);
}
console.log(`fonts:   ${fonts.length} woff2 inlined, weights ${inlinedWeights.join(',') || 'n/a'}` +
  ` (declared ${declaredWeights.join(',') || 'n/a'}) — port uses next/font`);
console.log(`source:  site-source.html ${template.length} bytes, ${template.split('\n').length} lines`);
console.log(`         site.css ${styles.slice(1).join('').length} bytes`);
console.log(`         site-logic.js ${scripts.join('').length} bytes`);
