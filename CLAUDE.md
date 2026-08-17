# CLAUDE.md — Samari Kuhsor Platform

Place this file at the **repository root**. It governs every session in this repo.

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
├── TASKS.md                   ← the task list. One task at a time; `make check` green before the next.
├── Makefile                   ← `make check` is the gate
├── docker-compose.yml         ← dev stack (Postgres 18)
├── docs/                      ← the specs below are ground truth, read them before building
│   ├── 01-DECISIONS.md        decisions already made — do not reopen these
│   ├── 02-SCHEMA.md           data model — authoritative
│   ├── 03-API-CONTRACT.md     endpoint and payload conventions
│   ├── 04-RBAC.md             permission model
│   ├── 05-MODULES.md          per-module functional spec
│   ├── 06-BUILD-PLAN.md       order of work — ordering authoritative, dates superseded by 07
│   ├── 07-IMPLEMENTATION-PLAN.md  HOW it is built: I1–I28, plus corrections C1–C6 to the docs above
│   └── reference/             client source material: clarification register, both ToRs
├── design/                    ← READ-ONLY. Approved prototypes. Never edit, never delete.
│   ├── Samari-Kuhsor-Green-CRM.html
│   ├── Samari Kuhsor - website.html
│   ├── HANDOFF-CRM-CONTEXT.md
│   └── PROJECT-CONTEXT-WEBSITE.md
├── backend/
│   ├── cmd/api/               entrypoint
│   ├── internal/
│   │   ├── auth/              sessions, password hashing, middleware
│   │   ├── rbac/              permission checks
│   │   ├── http/              handlers, routing, request/response types
│   │   ├── domain/<module>/   business logic per module
│   │   └── db/                sqlc-generated code (do not hand-edit)
│   ├── migrations/            goose migrations, sequential, never edited once applied
│   ├── queries/               hand-written SQL consumed by sqlc
│   └── sqlc.yaml
├── apps/crm/                  Next.js App Router
│   ├── app/api/               BFF route handlers — the ONLY caller of backend/
│   └── ...
├── apps/web/                  Next.js App Router (same BFF pattern)
└── packages/types/            shared TypeScript types, generated from the API contract
```

---

## 3. Stack

- **Backend:** Go, Postgres, [sqlc](https://sqlc.dev) for query codegen, [goose](https://github.com/pressly/goose) for migrations, `chi` for routing.
- **Frontends:** Next.js App Router, TypeScript, React.
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

## 4. Data rules — these are absolute

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

## 5. Design constraints — the client has already rejected alternatives

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

## 6. Language

- **UI text is Russian.** The interface ships with a ТҶ / РУ / EN switcher and all three are live
  at launch.
- **Code, comments, commit messages, and these docs are English.**
- Content — product names, descriptions, news, page copy — is **multilingual data**, not UI strings.
  It lives in `*_translations` tables keyed by locale `ru` | `tg` | `en`. Never hard-code a product
  name in a component.
- UI chrome strings live in a translation dictionary with the same three locales.

---

## 7. Testing

**Full unit and integration coverage is mandatory.** This was a deliberate decision; do not
downgrade it to save time.

- Every Go handler has integration tests against a real Postgres (testcontainers or a dedicated
  test database), covering the happy path, validation failure, and permission denial.
- Every domain function has unit tests. Ledger arithmetic, quarantine/release transitions and
  permission resolution get exhaustive case coverage.
- Every React data component has tests for loading, empty, error and populated states.
- A slice is not done until its tests pass. Do not open the next slice with a red suite.

---

## 8. Definition of done for a module slice

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

## 9. Things that are settled — do not reopen

Read `docs/01-DECISIONS.md` before proposing architecture. In particular: the stack is chosen, the
catalogue is exactly five products, there is no on-site Khorog instance at launch, there is no
offline mode, and Финансы и бюджет is deliberately out of scope for 9 September.
