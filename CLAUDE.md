# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## 1. What this is

**QOIM LLC** is a food and beverage producer in Тем, Хорог, ГБАО, Tajikistan. Its commercial brand
is **Самари Кӯҳсор / Samari Kuhsor** — fruit juices, jams, tomato paste and bottled drinking water.

This repository contains three applications:

| App | What it is |
|---|---|
| `backend/` | Go + Postgres API. The only thing that touches the database. Serves both frontends. |
| `apps/crm/` | Next.js internal CRM/ERP for factory and management staff. Also the CMS for the website. |
| `apps/web/` | Next.js public corporate website. Content comes from the CRM. |

> **Schedule superseded.** The original plan fixed launch at 9 September 2026. That deadline no
> longer governs the build. The sequence is now **build → internal test → deploy → client test over
> IP → DNS/TLS → launch**, driven by completion rather than dates. See `docs/07-IMPLEMENTATION-PLAN.md`
> and `TASKS.md`. Everything else in this file still stands.

The factory opens on launch day. Both systems are live when it does.

---

## 2. Repository layout

```
/
├── CLAUDE.md                  ← this file
├── TASKS.md                   ← T01–T38 with status. One task at a time; `make check` green before the next.
├── Makefile                   ← `make check` is the gate
├── docker-compose.yml         ← dev stack: db + api + seed + crm + web, all on 127.0.0.1
├── docker-compose.prod.yml    ← production topology: Caddy is the only service with a host port
├── playwright.config.ts       ← e2e, desktop + 390px mobile projects
├── docs/                      ← the specs below are ground truth, read them before building
│   ├── 01-DECISIONS.md        decisions already made — do not reopen these
│   ├── 02-SCHEMA.md           data model — authoritative
│   ├── 03-API-CONTRACT.md     endpoint and payload conventions
│   ├── 04-RBAC.md             permission model
│   ├── 05-MODULES.md          per-module functional spec
│   ├── 06-BUILD-PLAN.md       order of work — ordering authoritative, dates superseded by 07
│   ├── 07-IMPLEMENTATION-PLAN.md  HOW it is built: I1–I28, plus corrections C1–C6 to the docs above
│   ├── 08-REMAINING-PLAN.md   reasoning and test spec for T16–T38
│   ├── 09-RECOVERY-PLAN.md    R00–R21 — SUPERSEDES 08 and TASKS.md for outstanding work
│   ├── 10-CLIENT-NOTICE.md    draft written notice to QOIM (R19); not yet sent
│   └── reference/             client source material: both ToRs
├── design/                    ← READ-ONLY. Approved prototypes. Never edit, never delete.
├── tools/                     ← extractors (design/ → apps/) and the `make check` assertions
├── e2e/                       ← Playwright specs for the five ToR §5 workflows
├── deploy/                    ← Caddyfile, backup.sh, restore.sh, staged deploy README
├── backend/
│   ├── cmd/api|seed|devreset|analytics/
│   ├── internal/
│   │   ├── auth/              sessions, password hashing
│   │   ├── rbac/              permissions, Require middleware, route Registry + Verify
│   │   ├── audit/             the audit_log writer — called from the domain layer
│   │   ├── http/              handlers, routing, the rbac declarations; http/common/ envelopes
│   │   ├── api/               request/response DTOs — the source tygo generates from
│   │   ├── domain/<module>/   business logic per module
│   │   ├── db/                sqlc-generated code (do not hand-edit)
│   │   ├── seed/              reference (production-safe) and demo data
│   │   ├── alerts/            notification generation
│   │   └── testsupport/       the template-database integration harness
│   ├── migrations/            goose migrations, sequential, embedded via migrations/embed.go
│   ├── queries/               hand-written SQL consumed by sqlc
│   └── sqlc.yaml, tygo.yaml
├── apps/crm/                  Next.js App Router, port 3000
│   ├── app/api/               BFF route handlers — the ONLY caller of backend/
│   ├── app/<module>/          list, [id], [id]/edit, new
│   ├── lib/                   api.ts (server-only), resource.ts (client hooks), operations.ts
│   ├── messages/{ru,tg,en}.json   extracted from the prototype — regenerate, do not hand-edit
│   └── app/styles/            design tokens extracted from the prototype
├── apps/web/                  Next.js App Router, port 3001
│   ├── app/[locale]/          public pages
│   └── .reference/            recovered prototype source, for porting — not shipped
└── packages/types/api.ts      generated from Go DTOs by tygo. Never hand-edited.
```

---

## 3. Commands

Requires Go 1.26+, Node 24+, Docker Compose v2+, `goose`, `sqlc`, `tygo`.

```bash
make help                # every target
make up                  # Postgres 18 on 127.0.0.1:5433 (dev DB only)
make db-version          # must report 18.x — uuidv7() is a column default
make check               # THE GATE. Nothing moves on with it red.
make gen                 # extract + sqlc + tygo
make seed                # reference (production-safe) seed data
docker compose up --build   # the whole platform: web :3001, CRM :3000 (admin@samari-kuhsor.tj / DevPass!2026)
```

`make check` runs, in order: `go vet`, Go tests, prototype-extraction drift, `sqlc diff`, tygo
staleness, TypeScript across both apps, vitest, `next build`, then four assertions in `tools/`:
no backend URL or service key in the built client bundle, the production compose topology, the env
contract, and the Caddyfile.

**Running one test:**

```bash
cd backend && go test ./internal/domain/quality -run TestRelease -v
cd apps/crm && npx vitest run components/ItemsPage.test.tsx -t 'empty state'
npm run e2e -- --project=desktop e2e/sales.spec.ts   # stack must already be up + seeded
```

Playwright starts nothing: bring the stack up with `docker compose up --build`, seed `reference`
and `demo`, then run it. `E2E_PASSWORD` supplies the admin password.

**Dev-only escape hatch** — reset a forgotten local password:

```bash
APP_ENV=development DB_URL=... go run ./cmd/devreset user@example.tj 'NewPass!1'
```

---

## 4. Stack

- **Backend:** Go, Postgres 18, [sqlc](https://sqlc.dev) for query codegen, [goose](https://github.com/pressly/goose) for migrations, `chi` for routing, `pgx` + shopspring decimals.
- **Frontends:** Next.js App Router, TypeScript, React 19, TanStack Query, next-intl, Tailwind v4.
- **Auth transport:** httpOnly session cookie between browser and Next.js. The BFF attaches the
  service credential when calling Go.
- **Deployment:** single server in Dushanbe. Website and CRM on separate subdomains, one Postgres.

### Non-negotiable boundaries

- The browser **never** calls the Go API directly. Every request goes browser → Next.js route
  handler (BFF) → Go. No backend URL, token or service credential may appear in client-side code.
- Only `backend/` opens a database connection. Neither Next.js app has database access.
- Authorization is enforced in **Go middleware**, never in the BFF and never in React. The frontend
  may hide UI based on permissions, but hiding is not enforcement.

---

## 5. How those boundaries are actually enforced

Each rule in §4 has a mechanism. Work with the mechanism rather than around it.

**Every route declares a permission or is explicitly public.** Routes are mounted through
`rbac.Registry.Guarded(...)` or `Registry.Public(..., reason)` in `backend/internal/http/server.go`.
`rbac.Verify` then walks the real chi tree at startup and `NewServer` returns an error — the process
refuses to serve — if any route was registered without a declaration. A new endpoint that skips the
Registry will not boot. Actions are `read`/`manage`/`approve`; `manage` implies `read`, `approve`
implies nothing, and `approve` is only valid on the resources in `rbac.ApproveResources`.

**The audit row is written in the domain layer, inside the mutating transaction.** Pass the `tx`,
never the pool, to `audit.Record`. HTTP middleware cannot see the `before` state and a database
trigger cannot know an UPDATE was an *approval* — see the package comment in
`backend/internal/audit/audit.go` for why both alternatives were rejected. A failed audit write
rolls back the mutation, which is the correct direction.

**The credential boundary is a build failure, not a convention.** `apps/*/lib/api.ts` starts with
`import 'server-only'`, so a client component that imports it breaks the build. It is used only
inside `app/api/*`. `tools/check-bundle.mjs` re-checks the built output in `make check`.

**The BFF proxies; it does not decide.** `callApi` / `proxy` / `relay` / `download` in `lib/api.ts`
forward the query string whole, preserve the `{data, meta}` / `{error}` envelope and the status
code, and translate nothing — the frontend switches on the stable `error.code`. Validation, sorting
whitelists, paging clamps and permissions all live in Go; a second copy in the BFF is how the two
layers start disagreeing. The service key proves the caller is a BFF and is *not* an identity; the
user is the Bearer session token, which Go resolves itself. Never send a user id from the BFF.

**One CRUD engine, extracted from the reference slice.** `apps/crm/lib/resource.ts` gives every
module `useList/useOne/useCreate/useUpdate/useRemove/useAction`. Columns, KPIs and field groups are
deliberately *not* abstracted — they come from the approved prototype. `useUpdate` seeds the detail
cache from the response so the next edit sends the version the server just wrote; optimistic
concurrency is by `version`, and a stale one is a 409.

**Everything derived from `design/` is generated.** `tools/extract-website.mjs` and
`tools/extract-crm.mjs` produce `apps/crm/messages/*`, `apps/crm/app/styles/*`,
`apps/web/public/assets/*` and `apps/web/.reference/*`. Hand-editing those files is reverted by
`make check`, which re-runs the extractors and fails on any diff. Change `design/`? Impossible — it
is read-only. Change the extractor.

**Generated code is committed.** `packages/types/api.ts` (tygo, from `backend/internal/api`) and
`backend/internal/db` (sqlc, from `queries/`) are checked in, and `make check` fails when either is
stale. After touching a DTO or a query, run `make gen` and commit the output.

**Integration tests share one container.** `testsupport.NewDB(t)` clones a migrated template
database per test — one `postgres:18` testcontainer per binary, migrations applied once from the
same embedded FS the API uses. Do not wrap tests in an outer transaction: the domain layer opens
its own for audit writes and advisory locks.

---

## 6. Data rules — these are absolute

Derived from the synchronisation design agreed with the client. A second on-site instance in Khorog
and two-way sync are **phase 2**, but the schema is built for them now so phase 2 is additive
rather than a migration.

1. **Every table uses a UUID primary key.** Prefer UUIDv7 for index locality. Never `SERIAL`.
2. **Never store a computed balance as a column.** Stock and money are append-only movement
   ledgers; balances are derived by summation (materialised views are fine, source of truth is not).
   A row that says `quantity_on_hand = 500` must not exist.
3. **No hard deletes.** Every table carries `deleted_at timestamptz NULL`. Deleting sets the
   tombstone. Queries filter `deleted_at IS NULL`.
4. **Every table carries** `created_at`, `updated_at`, `version integer NOT NULL DEFAULT 1`,
   and `created_by uuid`. `version` increments on every update.
5. **Every mutation writes to `audit_log`.** Actor, action, resource, resource id, before/after,
   timestamp. This is a regulatory requirement, not a nicety.
6. **Money is `numeric(14,2)`. Never float.** Base currency is Somoni (TJS).
7. **Quantities are `numeric(14,3)`** to allow partial units of raw materials.

---

## 7. Design constraints — the client has already rejected alternatives

Both prototypes in `design/` are **client-approved**. They are the visual contract. Reproduce them;
do not improve them.

### CRM (`design/Samari-Kuhsor-Green-CRM.html`)

- **Green means _healthy_, never merely _branded_, inside the content area.** Status is its own
  axis: `ok` green `#1f7a3d`, `warn` amber `#b8791a`, `danger` red `#c0341c`, `info` grey.
  This was an explicit client instruction.
- Chrome — sidebar and top bar, fill `#124524` — is the only place green is decorative.
- Statuses are authored as data: `{t:'Label', v:'ok'|'warn'|'danger'|'info'|'neutral'}`. Never
  hard-code a colour on a tag.
- Sidebar is 252px; sidebar brand block and top bar are both exactly 64px so the divider is
  continuous across the seam.

### Website (`design/Samari Kuhsor - website.html`)

- **Keep the warm green/cream palette.** A cooler slate/turquoise variant was built and explicitly
  rejected. Page `#F5F7EE`, sections `#EAF1DD`, deep green `#23583A`, primary `#3E8E5A`,
  apricot accent `#E79A3A`.
- **Font is Golos Text**, chosen because it renders Tajik `ҳ`, `ҷ`, `ӯ` correctly. Do not substitute.
- **The conveyor belt is a plain green gradient** (`#4E8F63 → #2C5A3C`). A Pamiri textile pattern
  was tried and rejected.
- **The catalogue animation is v1** — products roll in from the left, stagger ~150ms, park in slots,
  batch buttons page between sets of four. A continuous-loop v2 was built and rejected.
- The assembly line stays **horizontal and swipeable on mobile**. It must never become a vertical list.
- Nothing cartoonish. Regional ornament is a light accent only.
- All major animations degrade under `prefers-reduced-motion` to static placement plus fades.

### Working rule

When asked for a change, change **only** that. The client has pushed back on unrequested redesigns
of adjacent areas. This applies to you as much as to the humans.

---

## 8. Language

- **UI text is Russian.** The interface ships with a ТҶ / РУ / EN switcher and all three are live
  at launch.
- **Code, comments, commit messages, and these docs are English.**
- Content — product names, descriptions, news, page copy — is **multilingual data**, not UI strings.
  It lives in `*_translations` tables keyed by locale `ru` | `tg` | `en`. Never hard-code a product
  name in a component.
- UI chrome strings live in a translation dictionary with the same three locales.
- **Tajik is `tg`, never `tj`** (C2). The prototype uses `tj`; the extractor renames it.
- Values the client has not yet confirmed render as `уточняется` (`orTBC` in `lib/resource.ts`),
  never as an empty cell — an empty cell reads as "none".

---

## 9. Testing

**Full unit and integration coverage is mandatory.** This was a deliberate decision; do not
downgrade it to save time.

- Every Go handler has integration tests against a real Postgres (`testsupport.NewDB`), covering the
  happy path, validation failure, and permission denial.
- Every domain function has unit tests. Ledger arithmetic, quarantine/release transitions and
  permission resolution get exhaustive case coverage.
- Every React data component has tests for loading, empty, error and populated states. `msw` fakes
  the BFF; `test/server-only-stub.ts` stands in for `server-only` under Vite.
- E2E specs **create their own preconditions** (`givenQuarantinedBatch` and friends in
  `e2e/fixtures.ts`) rather than consuming shared seed state — a spec that mutates what the next one
  reads fails in an order-dependent way that looks like a product bug. `permissions.spec.ts` asserts
  the server refuses, not merely that a button is hidden.
- A slice is not done until its tests pass. Do not open the next slice with a red suite.

A green domain test is not evidence the feature is reachable. `TASKS.md` was re-derived from code in
August 2026 after modules marked done turned out to have mutation hooks no component ever called.
Gates name a role, an action and a browser for that reason.

---

## 10. Definition of done for a module slice

A module is complete when all of the following exist:

1. Migration applied, schema matches `docs/02-SCHEMA.md`.
2. Queries in `queries/<module>.sql`, sqlc regenerated.
3. Go domain logic with unit tests.
4. Go HTTP handlers with RBAC middleware and integration tests.
5. BFF route handlers in the Next.js app.
6. Shared types in `packages/types`.
7. React list view matching the prototype, with the live search filter.
8. React detail view and edit form.
9. Component tests.
10. Seed data sufficient to demonstrate the module.
11. Audit log entries written for every mutation.

---

## 11. Things that are settled — do not reopen

Read `docs/01-DECISIONS.md` before proposing architecture. In particular: the stack is chosen, the
catalogue is exactly five products, there is no on-site Khorog instance at launch, there is no
offline mode, and Финансы и бюджет is deliberately out of scope for launch.

### Traps that have already cost time

- **Postgres 18 mounts at `/var/lib/postgresql`**, not `/var/lib/postgresql/data`. The old path
  makes the container refuse to start.
- **`Secure` cookies are not sent over plain HTTP.** `TLS_MODE` derives the flag (I24/I25); the dev
  stack runs `TLS_MODE=off` so login works on localhost.
- **Archivo has no Cyrillic** (C1).
- **Set `PUBLIC_SITE_URL` to the real domain before any wrappers are printed.** QR payloads embed
  it and wrappers are ordered months ahead (D11).
- **`seed demo` refuses to run in production.** Fabricated QC releases sitting in `audit_log` beside
  real ones would be a falsified regulatory record, and no-hard-delete means they cannot be removed.
