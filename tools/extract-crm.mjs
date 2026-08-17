// Extracts the approved CRM prototype into apps/crm.
//
// Two products:
//   1. The `T` translation object -> apps/crm/messages/{ru,tg,en}.json
//      The prototype keys Tajik as `tj`; the schema and this platform use `tg`
//      (ISO 639-1). See docs/07-IMPLEMENTATION-PLAN.md C2. The rename happens here.
//   2. Every :root custom property across the four CSS layers -> a Tailwind v4
//      `@theme` block. Values are COPIED, never re-derived (I12). Layer order is
//      load-bearing (HANDOFF-CRM-CONTEXT.md:394), so later layers overwrite earlier
//      ones exactly as the cascade would.
//
// Idempotent. Never writes into design/, which is read-only.
//
//   node tools/extract-crm.mjs

import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const PROTOTYPE = path.join(ROOT, 'design', 'Samari-Kuhsor-Green-CRM.html');
const MESSAGES = path.join(ROOT, 'apps', 'crm', 'messages');
const STYLES = path.join(ROOT, 'apps', 'crm', 'app', 'styles');

const html = fs.readFileSync(PROTOTYPE, 'utf8');

// ---------------------------------------------------------------------------
// 1. Translations
// ---------------------------------------------------------------------------

// Slice `var T={ … };` by brace balance rather than by regex — the object contains
// braces inside string literals and a naive match truncates it.
function sliceBalanced(src, startMarker) {
  const start = src.indexOf(startMarker);
  if (start < 0) throw new Error(`could not find ${startMarker}`);
  const open = src.indexOf('{', start);
  let depth = 0;
  let inStr = null;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (inStr) {
      if (c === '\\') i++;
      else if (c === inStr) inStr = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') inStr = c;
    else if (c === '{') depth++;
    else if (c === '}' && --depth === 0) return src.slice(open, i + 1);
  }
  throw new Error(`unbalanced object after ${startMarker}`);
}

const tSource = sliceBalanced(html, 'var T=');
const T = vm.runInNewContext(`(${tSource})`, Object.create(null), { timeout: 2000 });

const LOCALE_MAP = { ru: 'ru', tj: 'tg', en: 'en' }; // C2: tj -> tg
const locales = Object.keys(T);
if (locales.length !== 3 || !locales.every((l) => l in LOCALE_MAP)) {
  throw new Error(`unexpected locales in prototype: ${locales.join(', ')}`);
}

fs.mkdirSync(MESSAGES, { recursive: true });

// Structural equality check: a missing key in one locale is a silent blank in the UI.
function shapeOf(v, prefix = '') {
  if (v === null || typeof v !== 'object') return [prefix];
  if (Array.isArray(v)) return [`${prefix}[${v.length}]`];
  return Object.keys(v).sort().flatMap((k) => shapeOf(v[k], prefix ? `${prefix}.${k}` : k));
}
const shapes = Object.fromEntries(locales.map((l) => [l, shapeOf(T[l])]));
const reference = shapes.ru;
const shapeProblems = [];
for (const l of locales) {
  if (l === 'ru') continue;
  const missing = reference.filter((k) => !shapes[l].includes(k));
  const extra = shapes[l].filter((k) => !reference.includes(k));
  if (missing.length) shapeProblems.push(`${LOCALE_MAP[l]}: missing ${missing.join(', ')}`);
  if (extra.length) shapeProblems.push(`${LOCALE_MAP[l]}: unexpected ${extra.join(', ')}`);
}

const counts = {};
for (const [proto, locale] of Object.entries(LOCALE_MAP)) {
  const out = path.join(MESSAGES, `${locale}.json`);
  fs.writeFileSync(out, JSON.stringify(T[proto], null, 2) + '\n');
  counts[locale] = shapeOf(T[proto]).length;
}

// ---------------------------------------------------------------------------
// 2. Design tokens -> Tailwind v4 @theme
// ---------------------------------------------------------------------------

const styleBlocks = [...html.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]);
if (styleBlocks.length !== 4) {
  throw new Error(`expected 4 CSS layers, found ${styleBlocks.length}`);
}
const LAYER_NAMES = [
  'layer 1 — Modernist design system',
  'layer 2 — green palette override',
  'layer 3 — application layout',
  'layer 4 — green chrome + semantic status',
];

// Collect --custom-properties declared on :root, layer by layer. Later layers win,
// which is what the cascade does and why layer order is load-bearing.
const tokens = new Map(); // name -> { value, layer }
let declarations = 0;

styleBlocks.forEach((css, i) => {
  // Strip comments first: a commented-out token must not be extracted, and a
  // `:root` mentioned in prose must not be matched as a selector.
  const stripped = css.replace(/\/\*[\s\S]*?\*\//g, '');
  for (const block of stripped.matchAll(/:root\b[^{]*\{([^}]*)\}/g)) {
    for (const decl of block[1].matchAll(/(--[A-Za-z0-9_-]+)\s*:\s*([^;]+);/g)) {
      declarations++;
      tokens.set(decl[1], { value: decl[2].trim(), layer: i });
    }
  }
});

if (tokens.size === 0) throw new Error('no :root custom properties found — the parser is wrong');

// The ONLY place extracted values are altered. Every entry cites the correction in
// docs/07-IMPLEMENTATION-PLAN.md that authorises it. Anything not listed here is
// copied byte-for-byte from the client-approved prototype.
//
// C1: Archivo has no Cyrillic subset (latin, latin-ext, vietnamese only), so every
// Russian string in the prototype silently falls back to system-ui and Tajik is
// unrenderable. Browsers fall back per glyph, so inserting Golos Text — already
// chosen for this project because it renders ҳ, ҷ, ӯ correctly — leaves Latin in
// Archivo exactly as approved and fixes only the half that was never specified.
const OVERRIDES = {
  '--font-heading': {
    value: '"Archivo", "Golos Text", system-ui, sans-serif',
    reason: 'C1 — Archivo has no Cyrillic; per-glyph fallback to Golos Text',
  },
  '--font-body': {
    value: '"Archivo", "Golos Text", system-ui, sans-serif',
    reason: 'C1 — Archivo has no Cyrillic; per-glyph fallback to Golos Text',
  },
};

const applied = [];
for (const [name, o] of Object.entries(OVERRIDES)) {
  const existing = tokens.get(name);
  if (!existing) throw new Error(`override targets ${name}, which the prototype does not define`);
  if (existing.value === o.value) continue; // already correct; nothing to record
  applied.push({ name, from: existing.value, to: o.value, reason: o.reason });
  tokens.set(name, { value: o.value, layer: existing.layer, override: o.reason });
}

// CLAUDE.md §5 names these values as the design contract. Green means healthy,
// never merely branded; status is its own axis. If a future palette swap changes
// one of these, that is a client-visible regression and must fail loudly here
// rather than be discovered in review.
const REQUIRED = {
  '--color-accent': '#1f7a3d', // brand accent — green, NOT layer 1's red #ec3013
  '--sk-chrome': '#124524',    // sidebar + top bar fill, the only decorative green
  '--sk-ok': '#1f7a3d',
  '--sk-warn': '#b8791a',
  '--sk-danger': '#c0341c',
};
const contractBreaks = Object.entries(REQUIRED)
  .filter(([k, want]) => (tokens.get(k)?.value ?? '<missing>') !== want)
  .map(([k, want]) => `${k} = ${tokens.get(k)?.value ?? '<missing>'}, design contract requires ${want}`);

const byLayer = LAYER_NAMES.map((_, i) =>
  [...tokens.entries()].filter(([, t]) => t.layer === i)
);

const theme = [
  '/* Design tokens extracted from design/Samari-Kuhsor-Green-CRM.html.',
  ' *',
  ' * Generated by tools/extract-crm.mjs. DO NOT HAND-EDIT — re-run the script.',
  ' * Values are copied verbatim from the client-approved prototype (docs/07 I12).',
  ' * Where a later CSS layer overrode an earlier one, the later value is kept, which',
  ' * is what the cascade did in the prototype.',
  ' *',
  ' * Tailwind v4 reads --color-* / --font-* / --radius-* / --shadow-* and generates',
  ' * utilities from them. The remaining tokens stay available as plain CSS variables',
  ' * for the verbatim layer-1 primitives in components.css.',
  ' */',
  '',
  '@theme {',
  ...LAYER_NAMES.flatMap((name, i) => {
    const entries = byLayer[i];
    if (!entries.length) return [];
    return [
      `  /* ${name} */`,
      ...entries.flatMap(([k, t]) =>
        t.override
          ? [`  /* OVERRIDDEN — ${t.override} */`, `  ${k}: ${t.value};`]
          : [`  ${k}: ${t.value};`]
      ),
      '',
    ];
  }),
  '}',
  '',
].join('\n');

fs.mkdirSync(STYLES, { recursive: true });
fs.writeFileSync(path.join(STYLES, 'theme.css'), theme);

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------
console.log('translations → apps/crm/messages/');
for (const [locale, n] of Object.entries(counts)) {
  console.log(`   ${locale}.json${locale === 'tg' ? '   (renamed from prototype key "tj" — C2)' : '  '.padEnd(4)} ${String(n).padStart(4)} strings`);
}
if (shapeProblems.length) {
  console.log('\n   SHAPE PROBLEMS:');
  for (const p of shapeProblems) console.log(`     - ${p}`);
} else {
  console.log('   all three locales structurally identical');
}

console.log('\ntokens → apps/crm/app/styles/theme.css');
console.log(`   ${declarations} declarations across 4 layers → ${tokens.size} effective tokens`);
LAYER_NAMES.forEach((name, i) => {
  if (byLayer[i].length) console.log(`   ${String(byLayer[i].length).padStart(4)}  ${name}`);
});

// Tailwind v4 generates utilities from the --color-*, --font-*, --radius-* and
// --shadow-* namespaces. Anything else stays a plain CSS variable, which is what
// the verbatim layer-1 primitives consume.
const namespaces = {};
for (const k of tokens.keys()) {
  const ns = k.replace(/^--/, '').split('-')[0];
  namespaces[ns] = (namespaces[ns] || 0) + 1;
}
console.log('   namespaces: ' + Object.entries(namespaces)
  .sort((a, b) => b[1] - a[1]).map(([k, v]) => `--${k}-* (${v})`).join('  '));

if (applied.length) {
  console.log(`\ndeclared overrides (${applied.length}) — everything else is verbatim`);
  for (const o of applied) {
    console.log(`   ${o.name}`);
    console.log(`      from: ${o.from}`);
    console.log(`      to:   ${o.to}`);
    console.log(`      why:  ${o.reason}`);
  }
} else {
  console.log('\nno overrides applied — every token is verbatim');
}

if (contractBreaks.length) {
  console.error('\nDESIGN CONTRACT VIOLATION (CLAUDE.md §5):');
  for (const b of contractBreaks) console.error(`   - ${b}`);
} else {
  console.log(`\ndesign contract: ${Object.keys(REQUIRED).length}/${Object.keys(REQUIRED).length} required values verified`);
}

if (shapeProblems.length || contractBreaks.length) process.exit(1);
