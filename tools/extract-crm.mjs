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

// The prototype names two modules differently from the schema. The resource keys
// are canonical — they appear in role_permissions.resource, in audit_log.resource
// and in the permission list /auth/me returns (docs/04-RBAC.md §2) — so the
// message keys follow them, not the other way round. Same class of defect as C2:
// two vocabularies for one concept, silently diverging.
const MOD_KEY_MAP = {
  products: 'items',     // docs/04-RBAC.md §2 calls the module `items`
  website: 'inquiries',  // docs/04-RBAC.md §2 calls the module `inquiries`
};

function renameModuleKeys(dict) {
  if (!dict.mod) return { renamed: 0, dict };
  const mod = {};
  let renamed = 0;
  for (const [key, value] of Object.entries(dict.mod)) {
    const target = MOD_KEY_MAP[key];
    if (target) renamed++;
    mod[target ?? key] = value;
  }
  return { renamed, dict: { ...dict, mod } };
}

const counts = {};
let moduleRenames = 0;
for (const [proto, locale] of Object.entries(LOCALE_MAP)) {
  const { renamed, dict } = renameModuleKeys(T[proto]);
  moduleRenames += renamed;
  const out = path.join(MESSAGES, `${locale}.json`);
  fs.writeFileSync(out, JSON.stringify(dict, null, 2) + '\n');
  counts[locale] = shapeOf(dict).length;
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
// 3. Design-system primitives -> verbatim CSS
// ---------------------------------------------------------------------------
//
// docs/07-IMPLEMENTATION-PLAN.md I12 splits the prototype's CSS by risk:
//
//   design-system primitives (.btn, .tag, .card, .input, .table, .dialog, and the
//   .tag-ok/warn/danger/info status classes) carry the client's approval and
//   layer 1 is marked "do not edit" -> shipped VERBATIM
//
//   application layout (every .sk- class) is pure structure with no design
//   contract weight -> rewritten as Tailwind utilities
//
// The split is mechanical rather than a judgement call: a rule whose selectors
// are all .sk- is layout; anything else is a primitive. That means the boundary
// can be audited by re-running this script instead of by reading the diff.

const isLayoutRule = (selector) =>
  selector
    .split(',')
    .every((s) => /(^|[\s>+~])\.sk-/.test(s.trim()) || s.trim().startsWith('#sk-'));

// The prototype carries styling for all four original design directions
// (HANDOFF-CRM-CONTEXT.md:405). The client chose Modernist; classical, organic
// and nocturne were not built on. Their rules are inert here — nothing sets
// data-ds to them — but shipping them would import dead references to .sk-
// classes that are now Tailwind utilities.
const isRejectedDirection = (selector) =>
  /\[data-ds=["'](?!modernist)/.test(selector);

function extractPrimitives(css) {
  let stripped = css.replace(/\/\*[\s\S]*?\*\//g, '');
  // Drop block-less at-rules (@import, @charset). The prototype's
  // `@import url('https://fonts.googleapis.com/...')` is exactly the external
  // CDN dependency next/font replaces: a CRM on a single Dushanbe box must not
  // depend on Google Fonts being reachable from Khorog, and the font-family
  // itself comes from theme.css.
  // The URL contains semicolons (wght@400;600;800), so a naive [^;]* stops
  // inside it and leaves garbage that swallows the next rule.
  stripped = stripped
    .replace(/@import\s+url\([^)]*\)[^;]*;/g, '')
    .replace(/@import\s+(['"])(?:(?!\1)[\s\S])*\1[^;]*;/g, '')
    .replace(/@charset[^;]*;/g, '');
  const out = [];
  // Top-level rules only. Nested at-rules (@media, @supports) are carried whole
  // so their inner rules keep their context.
  const re = /(@[a-z-]+[^{]*\{(?:[^{}]|\{[^{}]*\})*\})|([^{}]+)\{([^{}]*)\}/g;
  let m;
  while ((m = re.exec(stripped))) {
    if (m[1]) continue; // at-rule: skipped, none of them carry primitives here
    const selector = m[2].trim();
    if (!selector || selector.startsWith(':root')) continue;
    if (isLayoutRule(selector)) continue;
    if (isRejectedDirection(selector)) continue;
    out.push(`${selector} {${m[3]}}`);
  }
  return out;
}

const primitives = [
  '/* Design-system primitives extracted verbatim from',
  ' * design/Samari-Kuhsor-Green-CRM.html.',
  ' *',
  ' * Generated by tools/extract-crm.mjs. DO NOT HAND-EDIT — re-run the script.',
  ' *',
  ' * These are the classes the client approved: .btn, .tag, .card, .input, .field,',
  ' * .table, .dialog and the semantic status tags. Layer 1 of the prototype is',
  ' * marked "do not edit" (HANDOFF-CRM-CONTEXT.md:66), so the rules ship byte for',
  ' * byte and visual fidelity is true by construction rather than by re-derivation.',
  ' *',
  ' * Every .sk- layout rule is deliberately excluded: that is application',
  ' * structure with no design-contract weight, and it is rewritten as Tailwind',
  ' * utilities (docs/07-IMPLEMENTATION-PLAN.md I12).',
  ' *',
  ' * Layer order is load-bearing (HANDOFF-CRM-CONTEXT.md:394): later layers',
  ' * override earlier ones, and they are emitted here in the same order.',
  ' */',
  '',
  '@layer components {',
  ...LAYER_NAMES.flatMap((name, i) => {
    const rules = extractPrimitives(styleBlocks[i]);
    if (!rules.length) return [];
    return [`  /* ${name} */`, ...rules.map((r) => '  ' + r.replace(/\n\s*/g, ' ')), ''];
  }),
  '}',
  '',
].join('\n');

// Guards. Each of these has already gone wrong once during development, and each
// failure was silent until a build broke or a CDN was quietly reintroduced.
for (const [pattern, why] of [
  [/@import/, 'an @import survived — the CDN font dependency is back'],
  [/:root\s*\{/, 'a :root block leaked into components.css; tokens belong in theme.css'],
  [/https?:\/\//, 'an external URL survived — nothing may be fetched at runtime'],
  [/\.sk-/, 'an .sk- layout rule leaked in; layout is Tailwind utilities (I12)'],
]) {
  const body = primitives.split('*/').slice(1).join('*/'); // ignore the header comment
  if (pattern.test(body)) {
    throw new Error(`extract-crm: ${why}`);
  }
}

fs.writeFileSync(path.join(STYLES, 'components.css'), primitives);
const primitiveCounts = LAYER_NAMES.map((_, i) => extractPrimitives(styleBlocks[i]).length);

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------
console.log('translations → apps/crm/messages/');
for (const [locale, n] of Object.entries(counts)) {
  console.log(`   ${locale}.json${locale === 'tg' ? '   (renamed from prototype key "tj" — C2)' : '  '.padEnd(4)} ${String(n).padStart(4)} strings`);
}
console.log(`   module keys renamed to match docs/04-RBAC.md §2: ${moduleRenames} ` +
  `(${Object.entries(MOD_KEY_MAP).map(([a, b]) => `${a}→${b}`).join(', ')})`);
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

console.log(`\nprimitives → apps/crm/app/styles/components.css`);
LAYER_NAMES.forEach((name, i) => {
  if (primitiveCounts[i]) console.log(`   ${String(primitiveCounts[i]).padStart(4)}  ${name}`);
});
console.log(`   ${primitiveCounts.reduce((a, b) => a + b, 0)} rules shipped verbatim; every .sk- layout rule excluded (I12)`);

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
