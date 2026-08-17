# TASKS — Samari Kuhsor platform

Dependency-ordered. **One task at a time. A task is not done until its "Done when" gate passes in
full. The next task does not open with a red suite.**

Governing docs: `CLAUDE.md` → `docs/01-DECISIONS.md` → `docs/07-IMPLEMENTATION-PLAN.md` → the
`docs/` file for the slice in hand.

Status: `todo` · `wip` · `done`

---

## Stage A — Foundations

### T01 · Repo scaffold and toolchain — `done`
Monorepo skeleton, git, Makefile, `.gitignore`, dev `docker-compose.yml` on `postgres:18`,
npm workspaces, Go module, `packages/types` placeholder.

**Done when:** `make up` starts Postgres 18 and `make db-version` reports `18.x`; `make down`
removes it cleanly; `make check` runs end to end and is green; `git log` has an initial commit.

### T02 · Extraction from the approved prototypes — `done`
Pull from `design/` (read-only, never edited):
- 4 images + Golos Text woff2 from the website bundle → `apps/web/public/`
- Recovered website source (markup, 282-line script, 13.7 KB CSS) → `apps/web/.reference/`
- CRM chrome strings (`T` object) → `apps/crm/messages/ru.json`
- Website RU copy → `apps/web/messages/ru.json`
- CRM `:root` tokens (layers ①②④) → `apps/crm/app/styles/theme.css` as Tailwind `@theme`

**Done when:** every asset byte-identical to the bundle source; both `ru.json` files parse and
contain no `tj` key (C2); token file contains every `--color-*`, `--space-*`, `--radius-*`,
`--shadow-*`, `--sk-*` custom property found in the prototype, values unchanged; a script asserts
the count matches.

**Result:** `tools/extract-website.mjs` + `tools/extract-crm.mjs`, both idempotent, both in
`make check` via `_check-extraction`.
- 5 assets recovered; `map-full.jpg` is byte-identical (md5) to the client's original `map.jpg`.
- Website source recovered to `apps/web/.reference/`: 1,073 lines markup · 17 KB logic · 5.8 KB CSS.
- **The prototype already contains all three languages, Tajik included** — 63 strings each,
  structurally identical. The CRM chrome does not need translating; only newly added strings do.
  This shrinks the D10 translation dependency to the website copy plus new UI.
- 79 token declarations across 4 layers → 59 effective (20 overridden by the green palette layer,
  including layer ①'s red `#ec3013` → `#1f7a3d`).
- Exactly **two declared overrides**, both C1 (`--font-heading`, `--font-body`); everything else
  verbatim. Overrides are printed on every run and annotated in the generated CSS.
- The extractor fails the build if any `CLAUDE.md §5` contract value drifts — verified by
  deliberately breaking `--sk-danger` and confirming a non-zero exit.

### T03 · Base migration and sqlc — `done`
Migration 001: universal conventions (`02-SCHEMA.md §1`), `users`, `sessions`, `roles`,
`role_permissions`, `user_roles`, `audit_log`, `items`, `item_translations`, `packaging_units`,
`item_prices`, `batches`. `uuidv7()` defaults (I7). Partial unique indexes for tombstones.

**Done when:** `goose up` then `goose down` round-trips cleanly; `sqlc generate` produces code with
no diff on re-run; a test asserts every table has `created_at`/`updated_at`/`deleted_at`/`version`/
`created_by`; a test asserts no column named `quantity_on_hand` or similar balance column exists.

**Result:** 11 tables. Verified against a live Postgres 18.6 —
- `goose down` leaves only `goose_db_version`; `goose up` restores all 11.
- Zero tables missing a convention column; `audit_log` correctly carries only its 9 exempt columns.
- Zero stored-balance columns.
- `uuidv7()` defaults produce genuine version-7 UUIDs.
- A `touch_row()` **trigger** owns `updated_at` and `version`, rather than a convention every
  handler must remember — a forgotten increment would silently disable optimistic concurrency on
  that table. Verified a caller cannot forge `version`: `SET version = 999` still yielded `OLD+1`.
- Tombstone frees the unique key: duplicate live SKU rejected, accepted after `deleted_at` is set.
- CHECK constraints reject bad enums — including **`locale = 'tj'`**, so C2 is enforced by the
  database, not by convention.
- sqlc → `decimal.Decimal` for money, `decimal.NullDecimal` for quantities, `uuid.UUID` for keys.
  Never float (CLAUDE.md §4.6/§4.7).
- The `sqlc diff` staleness gate was verified by adding a query without regenerating and confirming
  `make check` fails.

*Deferred to T11 by design:* the full Товары query set. `queries/items.sql` holds only what proves
generation.

### T04 · Test harness — `done`
`backend/testsupport`: one `postgres:18` testcontainer per run, migrations into a template DB,
`CREATE DATABASE … TEMPLATE` per test. The mandatory `AssertAudited` helper (I4).

**Done when:** two tests mutating the same table in parallel do not see each other's rows;
harness startup measured and recorded; `AssertAudited` fails loudly when no audit row was written.

**Result:** 7 tests green. Container startup **7.4s once per binary**; 13 database clones added
~110ms total, i.e. **~8ms per test** for full isolation.
- Isolation proven both sequentially and across 6 concurrent workers.
- The shopspring decimal codec is registered in `AfterConnect` and verified to round-trip
  `18.50` and `12.345` exactly — without it every numeric read fails, and it would fail late
  inside an unrelated module's test.
- `AssertAudited`/`AssertNotAudited`/`CountAudit` take a `TB` interface rather than `*testing.T`,
  so **their own failure paths are tested**: proven to fail on a missing row *and* on a duplicate.
  A silently-passing assertion would be worse than no assertion.
- `CREATE DATABASE … TEMPLATE` refuses to run while anything is connected to the template, so the
  explicit close after migrating is load-bearing, not tidiness.

### T05 · Auth — `done` (domain; HTTP endpoints land in T07)
argon2id, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, session token hashed at rest,
`failed_attempts`/`locked_until` lockout, idle timeout, absolute expiry, logout-everywhere on
password change.

**Done when:** integration tests cover — valid login issues a session; wrong password increments
`failed_attempts`; N failures lock the account; locked account rejects a *correct* password;
expired session rejected; revoked session rejected; `/auth/me` returns the flat permission list;
login and logout both write `audit_log`; the raw token never appears in the database.

**Result:** `internal/audit` + `internal/auth`, **33 tests green**. Every gate item covered, plus:
- `TestRawTokenIsNeverStored` enumerates *every* text column in *every* table and asserts the raw
  token appears in none of them — not just that `sessions.token_hash` looks like a digest.
- Deactivating a user takes effect on their **next request**, not their next login. A dismissed
  employee whose session stays valid until it lapses is a real access-control hole.
- A failed login writes **no** audit row (the counter increment still commits).
- The password-change audit entry is asserted to contain neither the plaintext nor any hash.
- Verification uses the parameters stored in the hash, so raising argon2 cost later cannot lock
  every user out.
- Unknown email still pays for a hash comparison, so login timing is not a user-enumeration oracle.

*Sequencing note:* this task delivers the auth **domain**. The `/auth/*` HTTP endpoints need the
response envelope and error mapping, so they are wired in T07 alongside the RBAC middleware.
*Test-design note:* lockout expiry is tested by expiring `locked_until` directly rather than by
sleeping — argon2 is deliberately slow, so a lockout short enough to wait for is shorter than the
failed attempts that trigger it, and such a test passes or fails on machine speed.

### T06 · RBAC — `done`
`rbac.Require(resource, action)`, permission resolution as the union across roles, `manage` implies
`read`, `approve` does **not** imply `manage`. Startup check failing if any registered route lacks
a permission declaration (`04-RBAC.md:123`).

**Done when:** unit tests for union, implication and no-roles-no-access; a test registers a route
without `rbac.Require` and asserts the server refuses to start; per-endpoint 200/403/401 matrix
helper exists and is used by T05.

**Result:** 22 tests green.
- The startup check is a **declaring router**, not middleware introspection: routes mount through
  `Registry.Guarded` or `Registry.Public`, and `Verify` walks the real chi tree and fails on
  anything registered outside it. Proven by mounting a `/secret-backdoor` route directly on chi.
- **`Public` requires a written reason.** That turns "this endpoint is unauthenticated" from
  something you get by forgetting a middleware into a decision someone had to justify.
- Startup panics rather than runtime 403s for: unknown resource, unknown action, duplicate
  declaration, and `approve` on a resource where `04-RBAC.md §3` does not define it.
- `ParsePermission` bug found by its own test: `":approve"` parsed cleanly because the *action*
  was valid, yielding an empty-resource permission that stores fine and matches nothing — it looks
  granted while granting nothing. Now rejected.
- Unknown *resources* are deliberately **not** rejected at parse time: an older binary meeting a
  resource key from a newer migration must not silently drop a user's other permissions. They are
  rejected where they are declared, at startup.

### T07 · Shared HTTP machinery — `done`
`internal/http/common`: response envelope, error mapping to the eight stable codes, pagination,
sort whitelisting, `q` search, `version` guard → 409 `version_conflict`. `internal/audit`.
`internal/alerts` skeleton (I15).

**Done when:** unit tests for every error code → status mapping; `per_page` clamped at 200;
unknown sort field rejected rather than interpolated; stale `version` returns 409; money and
quantity serialise as **strings** (`03-API-CONTRACT.md:147`), asserted by test.

**Result:** `internal/http/common` (33 tests) + `internal/http` + `internal/alerts`. Full backend
suite green. Beyond the gate:
- **Contract gap filled:** `03-API-CONTRACT.md:123` reserves **422** for business-rule violations
  but the code list at `:120` names no code for it. Added `business_rule`.
- Internal errors have their message *replaced* before sending, not merely omitted — an unexpected
  error is the one most likely to carry a SQL fragment or a file path. Asserted with a realistic
  leak.
- `DecodeJSON` rejects unknown fields and trailing JSON. A misspelled field would otherwise make an
  update appear to succeed while changing nothing.
- Sort is a **whitelist**, tested against injection, subqueries, column smuggling, and against a
  real-but-not-sortable column (`password_hash`) that would leak an ordering oracle.
- Empty collections serialise as `[]`, never `null`.
- Cyrillic is not `\u`-escaped on the wire.
- **`middleware.RealIP` deliberately not used** — deprecated for IP spoofing. `clientIP()` reads
  `X-Forwarded-For` explicitly, and only because nothing but the BFF can reach this port.
- The API sets **no cookie**: that is the BFF's job (I8), asserted by test.
- Login failures are byte-identical for wrong password / unknown address / deactivated account, so
  login is not a user-enumeration oracle. Lockout reports distinctly, because by then the account's
  existence is not a secret and the user needs to know why a correct password stopped working.
- **The startup check found a real bug in my own router:** `chi.Walk` reports full paths while the
  registry recorded sub-router patterns, so all four `/api/v1/*` routes read as undeclared. Fixed
  with `Registry.Scope`. The check earning its keep on day one is the best evidence it works.

### T08 · Type generation — `done`
`tygo` wired, `packages/types` generated from Go DTOs, staleness gate in `make check`.

**Done when:** `make gen` is idempotent; mutating a Go DTO without regenerating makes `make check`
fail with a clear message.

**Result:** `internal/api` is the single source of truth; `packages/types/api.ts` is generated by
`tygo`. Verified by adding a field to a Go DTO without regenerating and confirming `make check`
fails, then reverting.
- `common.Money` / `common.Quantity` map to TypeScript **`string`**, so the "never a JSON number"
  rule is expressed in the type system rather than in a comment nobody reads.
- The HTTP handlers were refactored onto these types, so no payload is defined twice — defining
  them in both places is exactly the drift `03-API-CONTRACT.md:265` warns about.

### T09 · `seed:reference` — `done`
Five products with packaging units and translations, seed roles with the exact permission matrix
from `04-RBAC.md §4`, locations, one admin user, content page skeletons. Idempotent.

**Done when:** running twice produces no duplicates and no error; a test asserts the seeded
permission matrix equals `04-RBAC.md §4` cell for cell; a test asserts exactly five finished goods
exist and none of the prototype's filler SKUs (D8).

**Result:** 5 roles, 59 permissions, 5 items, 5 translations, 7 packaging units, 1 admin. Second
run: all zeros. Verified against the live dev database *and* end to end through the running API.
- The matrix test **restates `04-RBAC.md §4` independently** rather than reading the seed's own
  data, so a typo shows up as a disagreement instead of a role quietly missing a permission.
- A dedicated test asserts **Директор has `manage` on nothing** — `04-RBAC.md:95` ("management
  reads; the floor writes") is a deliberate design decision that must survive someone later
  assuming a director should be able to edit everything.
- Shelf life, composition and nutrition are asserted **null**. Seeding a plausible value would
  publish an unverified claim, which the client explicitly forbade (`02-SCHEMA.md:176`).
- Only `ru` translations are seeded. Inventing Tajik or English product names would publish
  unreviewed content; a missing locale row correctly falls back to `ru`.
- Filler SKUs are asserted absent by code *and* by name (гранат / клубнич / 1,5 л).
- The admin password is **generated** when unset and printed once, never defaulted — a well-known
  default password on a system holding regulatory records is worse than no seed at all.

*Deferred:* `locations` (needs T16) and content page skeletons (need T28).

### T10 · CRM shell — `done`
Next 16 + React 19 + Tailwind v4 + next-intl (cookie mode) + TanStack Query. Sidebar 252px, top bar
64px, permission-driven nav, global search, ТҶ/РУ/EN switcher, notification bell, user menu. BFF
auth route handlers and the httpOnly cookie flow.

**Done when:** login works browser → BFF → Go → Postgres and back; nav hides modules the user
cannot `read`; language switch changes all chrome strings; **screenshot drift gate** (I13) run
against the prototype at 1440px; responsive verified at tablet and mobile (I27); component tests
for loading/empty/error/populated.

**Result:** Next 16.3.1 + React 19.2.8 + Tailwind 4.3.3 + next-intl 4.13 + TanStack Query 5.101.
10 component tests; full `make check` green.
- Login verified end to end in a real browser: browser → BFF → Go → Postgres → back.
- **The session cookie is invisible to JavaScript**, verified in-browser (`document.cookie` is
  empty while authenticated) rather than merely asserted in code.
- **The drift gate found two real defects on its first run** — see below.
- `lib/api.ts` opens with `import 'server-only'`, so a client component importing it **fails the
  build** rather than shipping `BACKEND_URL`/`SERVICE_KEY` to the browser.
- `tools/check-bundle.mjs` scans the built client assets for the service key, the backend URL and
  the `/api/v1/` prefix. Proven by planting a leak, watching the gate fail, and reverting.
- The four required states live in the shell, because the session query is what every screen
  depends on. "Empty" is real: an administrator can create a user and forget to assign a role.
- The logout BFF route clears the cookie **even if the API call fails** — a user who pressed
  «выйти» on a shared factory terminal must end up logged out regardless.

**Two drifts the screenshot gate caught:**
1. **Locale switcher order.** Mine rendered `РУ ТҶ EN`; the approved prototype is `ТҶ РУ EN`.
2. **The user chip showed the role *key*** (`admin`) instead of its name. `roles` carries
   `name_ru/name_tg/name_en` (`02-SCHEMA.md:56`) precisely so it need not — but `/auth/me` was
   returning bare strings. **Contract changed:** `api.Role` now carries all three names, because a
   role name is *content* an administrator writes, not UI chrome (`CLAUDE.md §6`).

*Deferred:* nav count pills need `alerts` queries (T16+); the responsive pass (I27) lands with the
first module that has a table to reflow.

---

## Stage B — Reference slice

### T11 · Товары — backend — `done`
`queries/items.sql`, domain logic, handlers with `rbac.Require`, audit on every mutation.

**Done when:** integration tests for happy path, validation failure, 403, 401, audit row asserted;
duplicate SKU on a non-deleted row rejected with `already_exists`; tombstoned SKU may be reused.

**Result:** 54 integration tests + 20 domain unit tests. Full suite green.
- The **permission matrix runs as a table across all five endpoints** × three tokens
  (`items:manage`, `items:read`, no roles) × unauthenticated. That table is what the next eleven
  modules copy, so `read` failing to imply `manage` is proven per endpoint, not once.
- **A rejected mutation writes no audit row** — asserted, so the trail never records work that
  did not happen.
- Duplicate SKU is a **400 with `already_exists`**, and the response is asserted not to leak
  `23505`, `pq:` or the constraint name.
- `sort` is a whitelist tested against `password_hash` — a real column whose sortability would leak
  an ordering oracle.
- **D8 SKU prefixes enforced**: `RAW-` and `PKG-` required; finished goods exempt because the five
  approved codes share no prefix. All eleven cases tested.
- Search matches **every locale**, so a Tajik-speaking operator finds a product by its Tajik name
  while the list renders in Russian.
- Unverified claims (composition, nutrition, shelf life) asserted **null**, not `""` — the UI needs
  null to render «уточняется» rather than publish an unverified claim.
- A test asserts the API **never exposes a stock quantity** under any name, so a balance column
  appearing later fails here rather than shipping.

**A design decision worth flagging for review:** the list query originally used a
`LEFT JOIN LATERAL` for the current price. sqlc infers lateral columns as **non-null**, so an item
created before it was priced — the normal order of work — would fail to scan at runtime. Replaced
with three batch queries per page, constant in page size.

*Two test defects found and fixed:* a substring assertion on `jsonb` (Postgres reformats it with
spaces) and an anonymous struct missing `json` tags so `per_page` never unmarshalled. Both were my
tests being wrong, not the product.

### T12 · Товары — BFF and types — `done`
**Done when:** no backend hostname, port or service key appears in any client bundle — asserted by
a test grepping the built output; `packages/types` current.

**Result:** Six BFF routes behind one `proxy()` helper — the shape eleven modules copy. The query
string is forwarded **whole** rather than allow-listed: Go already validates every parameter, and a
second drifting copy of those rules in the BFF is how the two layers start disagreeing about what
is valid.

**Contract bug found by the gate:** tygo maps a Go pointer to an *optional* property (`field?: T`),
but the API marshals a nil pointer as `"field": null` — `03-API-CONTRACT.md:216` shows
`"barcode": null` explicitly. The TypeScript claimed `undefined` where the wire carries `null`.
Fixed with per-field `tstype:"… | null"` tags on **response** DTOs only; request DTOs keep `?`,
because a POST genuinely does omit `version`.

### T13 · Товары — UI — `done`
List (KPIs, columns, live search), detail per `05-MODULES.md §2`, edit form with `version_conflict`
handling.

**Done when:** component tests for four states; a stale-version save surfaces a conflict rather than
overwriting; `уточняется` renders for null composition/nutrition/shelf-life (`02-SCHEMA.md:176`);
screenshot drift gate; responsive pass.

**Result:** 22 new component tests (32 total in the CRM). `ListView`, `DetailView`, `StatusTag` and
`ItemForm` are all module-agnostic — they are what T15 extracts.
- **`StatusTag` never maps a status key to a colour.** It maps `level` → the prototype's verbatim
  `.tag-*` class, so green means *healthy* and not merely *branded* (`CLAUDE.md §5`). Tested for
  every level, plus an unknown level falling back to neutral rather than rendering unstyled.
- **A 409 is surfaced as an actionable conflict and the save button is disabled** — never retried.
  Retrying a version conflict overwrites exactly what the guard protected.
- Field errors render **against their input**, keyed on the API's stable field codes.
- The empty state distinguishes "no products" from "no matches" — "add your first product" is
  unhelpful when the answer is "clear the search box".
- **The form does not offer composition or nutrition inputs at all.** Offering them invites a
  plausible guess, and the client forbade publishing unverified claims.
- SKU and item type are locked when editing, with the reason shown: batches and stock movements
  reference them.

### T13 · Товары — UI — `todo`
List (KPIs, columns, live search), detail per `05-MODULES.md §2`, edit form with `version_conflict`
handling.

**Done when:** component tests for four states; a stale-version save surfaces a conflict rather than
overwriting; `уточняется` renders for null composition/nutrition/shelf-life (`02-SCHEMA.md:176`);
screenshot drift gate; responsive pass.

### T14 · QR generation and printer export — `done`
Writes `batches.qr_payload`, generates images on demand, bulk export as ZIP of SVGs + CSV manifest
(D11, I17).

**Done when:** payload round-trips through a QR decoder in test; export produces one SVG per batch
and a manifest whose row count matches; regenerating a payload for an already-issued batch is
refused.

**Result:** 16 domain tests + 9 integration tests. Everything here is shaped by one fact from D11 —
**a printed wrapper cannot be corrected.**
- **Re-issuing is refused (422), never silently overwritten.** A second payload would invalidate
  wrappers that may already be printed and in transit. Guarded twice: a read check *and*
  `WHERE qr_payload IS NULL` in the UPDATE, so two concurrent requests cannot both succeed.
- Issuing is audited as **`approve`**, not `update` — it commits the company to a wrapper order,
  and the trail should name who decided that.
- **The payload is a URL, not encoded production data**, asserted. A wrapper printed in August
  cannot learn the batch was recalled in October, so the code must resolve at scan time.
- **SVG, not PNG** — the printer scales to the wrapper die, and a raster at the wrong size either
  pixelates or costs a re-request.
- The 4-module **quiet zone** is included and asserted: the QR spec requires it, print houses don't
  add it, and without it scanning fails against a busy wrapper.
- The manifest is **semicolon-delimited with a UTF-8 BOM** — the printer opens it in Excel, and a
  comma-separated UTF-8 file renders Cyrillic as mojibake in a Russian locale.
- Manifest row count is asserted against SVG file count: **a short export means a batch ships with
  the wrong wrapper.** Unissued batches are asserted absent.
- The export is **buffered, not streamed**: a mid-stream failure after the status line would hand
  the printer a truncated ZIP that looks complete.

**A real bug the tests caught:** expiry-before-production returned **500** — the CHECK constraint
fired and was never mapped. Now validated in the domain with a field-named message, plus a
constraint-name fallback so no constraint can surface as a 500.

*Also fixed:* I wrote a test that asserted nothing (`_ = wantLevel`). Replaced with a real
assertion, which required adding `GET /batches/{id}` — an endpoint the item detail view needs
anyway.

### T15 · Engine extraction — `done`
Extract `ListView` / `DetailView` / `EditForm` and their per-module descriptors **from the working
Товары code** (I2). Товары is refactored onto the engine as consumer zero.

**Done when:** Товары's tests and its screenshot drift gate still pass unchanged after refactor.

**Result:** `lib/resource.ts` — `createResourceHooks(resource)` returning list/one/create/update/
remove/action. Товары refactored onto it; **its 32 tests passed with zero changes to the test
files**, which is the gate. 13 new tests cover the engine directly, since a bug there is a bug in
eleven modules at once.

`lib/items.ts` went from ~150 lines to ~55: a resource name, two row types, and the filter mapping.
Склад's version should be about that long.

**Deliberately not abstracted:** columns, KPIs and field groups (they differ by design and come
from the approved prototype — a config DSL would be a worse way to write JSX), validation and
business rules (those live in Go), and anything whose only second consumer is hypothetical.

The engine test that matters most: **`useUpdate` seeds the detail cache from the response.**
Invalidating alone leaves a window where the form still holds the old version and 409s against the
user's own previous save — which looks like the app randomly refusing to save.

**A real bug the visual check caught:** the list showed "Всего SKU —" while "Активных" showed 5.
`relay()` in the BFF was re-wrapping only `data` and **dropping `meta`** — so pagination metadata
never reached the browser and paging was broken too. The component tests mock `/api/items` directly,
so nothing exercised the BFF. Fixed, and `lib/api.test.ts` now covers that layer: meta passthrough,
error-envelope preservation, 204 handling, no user identity ever sent, and an unreachable backend
reported without leaking its address.

---

## Stage C — The operational chain

### T16 · Склад и запасы — `todo`
`locations`, `stock_movements`, `stock_balances` **plain view** (I5). Advisory-lock negative
posting with `adjustment` exempt (I6). Приёмка / перемещение / списание / корректировка.

**Done when:** exhaustive ledger tests — balance equals sum of deltas; transfer posts two rows
sharing `ref_id` and nets to zero; correction is a compensating entry and the original row is
untouched; concurrent negative postings cannot drive stock negative; `adjustment` *can*; no endpoint
anywhere accepts an absolute quantity.

### T17 · Производство — `todo`
`manufacturing_orders`, `production_entries` (append-only). Completion posts `production_output`
into a quarantine location and moves the batch to `quarantine`.

**Done when:** yield/output/downtime are computed sums, never columns — asserted; completing an
order does **not** make the batch sellable; MO↔batch is 1:1.

### T18 · Качество и безопасность — `todo`
`quality_tests`, `batch_status_events`. Transition rules from `02-SCHEMA.md §7`.
**Reviewed line by line.**

**Done when:** the full from/to transition matrix is tested — every legal pair, every illegal pair,
with and without `quality:approve`; recall (`released → rejected`) requires a reason; release writes
an immutable event plus an audit row naming the deciding user; a sales order and a shipment line
both reject a non-`released` batch, enforced in the domain and proven by test.

### T19 · Закупки и поставщики — `todo`
Suppliers, POs, lines, goods receipts. `procurement:approve` gates exit from `approval`.

**Done when:** goods receipt posts `goods_receipt` movements matching received quantities; approval
without the permission is 403; receiving against a closed PO is refused.

### T20 · Интеграция с сайтом — `todo`
`inquiries`, reference-number generation per prefix, convert-to-lead.

**Done when:** each type produces its correct prefix; reference numbers are unique under concurrent
submission; a `CP-` complaint must link to a batch; conversion carries the reference number across.

---

## Stage D — Remaining modules

### T21 · Логистика — `todo`
**Done when:** loading a shipment line with a non-`released` batch is refused server-side.

### T22 · Документы — `todo`
**Done when:** superseded versions are retained; `documents:approve` gates `approval → active`;
document files are unreachable by any static path and require `documents:read` (I17).

### T23 · Персонал — `todo`
**Done when:** personal data is unreachable through every public endpoint, asserted by test;
contract expiry warns at 30 days.

### T24 · Оборудование и ТО — `todo`

### T25 · CRM и продажи — `todo`
Customers, contacts, leads, deals with `deal_stage_events`, sales orders, tasks (I16).

**Done when:** confirming a sales order posts `sale` movements and refuses non-`released` batches.

### T26 · Role management UI and audit log viewer — `todo`
`04-RBAC.md §6`. Behind `admin:manage` and `audit:read`.

**Done when:** the last holder of `admin:manage` cannot be deactivated or stripped of it, enforced
server-side and tested; permission changes take effect on the affected user's next request;
audit rows are not editable or deletable through any route.

### T27 · Notifications — `todo`
Derive 7 standing conditions, persist 3 discrete events (I15). Sidebar count pills from the same
service.

**Done when:** a resolved condition disappears from the feed with no retraction logic; users never
see a notification for a resource they cannot `read`.

---

## Stage E — Website

### T28 · CMS — `todo`
`content_pages`, `content_blocks`, translations, `news_posts`, `media`, `content_workflow_events`.
Ladder: draft → technical_review → language_review → approved → published.

**Done when:** every illegal transition is refused; `approved`/`published` require `cms:approve`;
the public API returns only `published`; the CRM can preview any state.

### T29 · Website port — `todo`
1:1 translation of the recovered source (I19). CSS verbatim. Content from CMS/`items`.

**Done when:** side-by-side screenshot comparison against the prototype at desktop, tablet and
mobile; belt roll-in, batch paging, map three-stage draw, marquee and replay-on-return all behave
as `PROJECT-CONTEXT-WEBSITE.md §7` describes; `prefers-reduced-motion` degrades to static placement
plus fades; the assembly line stays horizontal and swipeable on mobile.

### T30 · Public endpoints and inquiry submission — `todo`
**Done when:** submission returns a reference number; rate limiting by IP proven; the inquiry
appears in the CRM as `new`; no session is ever required; the public surface cannot reach any
CRM endpoint.

### T31 · next-intl routed locales, hreflang, legal pages — `todo`
**Done when:** `/ru`, `/tg`, `/en` all render; `hreflang` present and correct; missing locale rows
fall back to `ru`.

### T32 · Matomo, consent banner, retention — `todo`
**Done when:** no analytics request fires before consent is given, asserted in a browser test.

### T33 · Панель управления — `todo`
Built last (`05-MODULES.md:60`).

**Done when:** Дебиторка card hidden; Выручка sourced from confirmed sales orders only; empty
states render rather than zeros or sample data (`05-MODULES.md:70`); period switch re-plots the
chart and the revenue KPI only.

---

## Stage F — Verification and delivery

### T34 · Full internal test pass — `todo`
**Done when:** `go test ./...`, `sqlc diff`, `tygo` staleness, `vitest` ×2, `next build` ×2 and
**Playwright E2E across the five ToR workflows** (I26) are all green; the production cookie
assertion under `TLS_MODE=auto` passes (I25); responsive pass complete across all modules.

### T35 · Staging rehearsal — `todo`
**Done when:** clean-box deploy from the compose file succeeds; `seed:reference` runs; **restore
test restores both `pg_dump` and the `uploads` tar** and a document opens afterwards (I18).

### T36 · Server deploy for client testing — `todo`
`TLS_MODE=off`, two ports, plain HTTP, IP access (I25).

**Done when:** both systems reachable; login works; `seed:demo` refuses to run.

### T37 · Client feedback — `todo`
Absorbed as ordinary slices.

### T38 · DNS, TLS, launch — `todo`
`TLS_MODE=auto`, host-based routing, Let's Encrypt.

**Done when:** both subdomains serve valid certificates; no application code changed to get there;
staff accounts created; Russian training materials delivered; the D7 offline limitation is recorded
to QOIM in writing.
