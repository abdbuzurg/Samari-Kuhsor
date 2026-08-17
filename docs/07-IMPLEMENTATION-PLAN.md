# 07 — Implementation Plan

How the platform gets built.

`01`–`06` settle **what** to build. This file settles **how**, and records corrections where the
existing docs contradict each other or contradict reality. Read it after `01-DECISIONS.md`.

> **Schedule superseded.** `06-BUILD-PLAN.md §3` sequences the work against a fixed 9 September 2026
> launch. That calendar no longer governs: the work is now **build → internal test → deploy →
> client test → DNS/TLS → launch**, driven by dependency order and completion, not by dates.
> `06-BUILD-PLAN.md`'s *ordering* logic remains authoritative; its *dates* do not. See §7.

Decisions here are prefixed `I` (implementation) to keep them distinct from the client-facing `D`
decisions in `01-DECISIONS.md`. They are settled on the same terms: do not reopen, do not silently
implement something different.

Last updated: 17 August 2026.

---

## 1. Corrections to ground truth

These amend `01`–`06`. Where this section conflicts with an earlier doc, **this section wins** —
each item below is a verified defect, not a preference.

### C1 — Archivo cannot render Cyrillic

`design/Samari-Kuhsor-Green-CRM.html:9` loads Archivo. Its Google Fonts subsets are `latin`,
`latin-ext`, `vietnamese`. **No Cyrillic.** Every Russian string in the approved CRM prototype is
falling back to `system-ui`; only the Latin codes (`APJ-1000`, `MO-0612`, `B-2617`) are actually
Archivo. Tajik — `ҳ ҷ ӯ қ ғ ӣ`, which live in `cyrillic-ext` — is unrenderable.

This is the same defect the website already diagnosed and fixed by choosing Golos Text
(`PROJECT-CONTEXT-WEBSITE.md:41`). The CRM never got the cure.

**Resolution:** `font-family: "Archivo", "Golos Text", system-ui, sans-serif`. Browsers fall back
per glyph, so Latin renders in Archivo exactly as approved and Cyrillic resolves to a face vetted
for Tajik. Both self-hosted via `next/font`. Nothing the client approved changes.

### C2 — The Tajik locale code is `tg`, never `tj`

The prototype uses `tj` (`design/Samari-Kuhsor-Green-CRM.html:518`). The schema uses `tg`
(`02-SCHEMA.md:45`). `tg` is ISO 639-1; `tj` is a country TLD. **The schema is right.**

Dictionary keys, cookie values, URL segments and every `locale` column use `tg`. The display label
stays `ТҶ`. Any occurrence of `tj` as a locale code is a bug.

### C3 — Status labels and error messages must not be Russian in the payload

`03-API-CONTRACT.md:174` ships `"label": "Карантин"` and `:117` states `message` is Russian. D10
puts the CRM in three languages, so as written a Tajik interface shows Russian status tags and
Russian validation errors.

**Resolution — the backend owns semantics, the frontend owns words.** The API sends
`status.key` + `status.level` and `error.code` + `details[].code`. All user-visible text is rendered
from the i18n dictionary. The Russian `label` and `message` stay in the payload as fallbacks so
nothing renders blank on a missing key.

`03-API-CONTRACT.md:177` — never let React map a status to a colour — is fully preserved: `level`
remains server-decided. Only the *word* moves.

Consequence, and the reason this matters this week: **every user-visible string ends up in one set
of JSON files**, which is what goes to the translators (D10, the longest lead time in the project).

### C4 — The website prototype is already React; D5's estimate is misallocated

`design/Samari Kuhsor - website.html` is a compiled bundle declaring `react@18.3.1` and
`react-dom@18.3.1`. Recovered contents:

| Piece | Size |
|---|---|
| Declarative markup (`<x-dc>` template) | 1,073 lines |
| Logic + content data | 282 lines / 17 KB |
| CSS | 13.7 KB, two blocks |
| Assets | 4 images + Golos Text woff2, all extractable |

Animations are **state-driven CSS** (`beltPos`, `animCss`, SVG `pathLength`), not imperative DOM.

D5 budgets 70–110h to "rebuild imperative animations around refs and effects." That describes the
**CRM** prototype, which is genuinely vanilla. The website port is a mechanical translation of
~1,300 lines.

### C5 — Postgres 18 is a hard production requirement

`02-SCHEMA.md:16` asks for UUIDv7. Native `uuidv7()` landed in **Postgres 18**. Per I7 the platform
runs PG18 and uses it as the column default. The Dushanbe server must therefore run PG18 — confirm
this before the deploy stage, not during it.

### C6 — Schema gaps

`05-MODULES.md` references entities `02-SCHEMA.md` does not model. Resolution per I16:

| Gap | Resolution |
|---|---|
| `tasks` (CRM KPI "Просроченные задачи") | Add |
| `contacts` (customer detail) | Add |
| deal stage history | Add `deal_stage_events`, mirroring `batch_status_events` |
| `notifications` (10 triggers, §17) | Add — but only for 3 of them, see I15 |
| BOM / material reservation | **Not built.** `06-BUILD-PLAN.md:92` already softens the ToR flow to "material issue", so acceptance is unaffected. Largest single scope saving available. |
| Funnel stage "Оплачено" | Hidden — needs `invoices`, deferred with finance. Same treatment `05-MODULES.md:65` already prescribes for Дебиторка. |

---

## 2. Implementation decisions

| # | Decision |
|---|---|
| **I1** | Claude Code authors backend, frontend and tests. Review happens at **slice boundaries** against the `06-BUILD-PLAN.md §1` checklist — line-by-line only for the Товары reference slice and the whole `quality` module. |
| **I2** | Товары is built **fully concrete**, reviewed, and the list/detail/edit engines are then **extracted from working code** before module 2. Склад is consumer #1. No abstraction ahead of evidence. |
| **I3** | API contract is **code-first**. Go DTOs are the source of truth; TypeScript generated by `tygo`; a `make check` gate fails on stale output. Shared `internal/http/common` for envelope, list params, version guard and audit. Handlers hand-written per module — no generic CRUD factory, because the ledger, quality transitions and goods receipt do not fit one. |
| **I4** | `audit_log` is written **explicitly in the domain layer, inside the mutation's transaction**, via `audit.Record(ctx, tx, …)`. Only the domain layer knows an update was an *approval*. Enforced by a mandatory assertion in the shared integration-test harness. |
| **I5** | `stock_balances` is a **plain SQL view**, not materialised. Always exact, no refresh machinery, no staleness class of bug. The name is preserved so a materialised swap later is invisible to callers. |
| **I6** | Any transaction posting a **negative** stock delta takes `pg_advisory_xact_lock` on `(item_id, batch_id, location_id)` and re-reads the balance inside the lock. Going negative is rejected `422` — **except `reason = 'adjustment'`**, which must be permitted to go negative because it is the correction mechanism. |
| **I7** | **Postgres 18** in dev, staging, test and production. `uuidv7()` as the column default. |
| **I8** | BFF → Go: `Authorization: Bearer <session token>` (opaque, hashed at rest) plus a static `X-Service-Key`. Go resolves user + union of role permissions in **one query per request, uncached** (`04-RBAC.md:148`). The `api` container publishes **no host port**. The BFF must never send a user id — identity resolution lives only in Go. |
| **I9** | **CRM**: client components + TanStack Query against its own BFF. Its query states map 1:1 onto the four mandated component-test states. **Website**: Server Components + ISR fetching Go server-side, with a deliberately narrow BFF for inquiry POST and catalogue filtering, and `'use client'` islands for animations. |
| **I10** | Tests: **one `postgres:18` testcontainer per run**, migrations applied into a template database, `CREATE DATABASE … TEMPLATE` per test for isolation. Vitest + RTL + MSW on the frontend. The **five ToR workflows are Go API-level integration tests**, written incrementally as modules land. **Plus Playwright E2E** driving the same five workflows through a real browser (I26) — the API tests prove the business rules, the browser tests prove the chain a user actually walks. |
| **I11** | CRM typography per C1. |
| **I12** | CRM styling: **Tailwind v4**. Prototype `:root` tokens auto-extracted into `@theme` (values copied, never re-derived). Design-system layer ① primitives (`.btn`, `.tag`, `.card`, `.input`, `.field`, `.seg`, `.table`, `.dialog`) kept as **verbatim CSS** in `@layer components` — they carry the client's approval and are marked do-not-edit. All `sk-` layout becomes utilities. |
| **I13** | A **screenshot drift gate**: the prototype and the built CRM are rendered at 1440px and compared before any slice is called done. Fidelity becomes an observation, not a judgement call. |
| **I14** | i18n: `next-intl` in both apps. **Routed** (`/ru`, `/tg`, `/en` + `hreflang`) on `apps/web` for SEO; **unrouted, cookie-driven** on `apps/crm`. Messages as JSON preserving the prototype's `T` shape. |
| **I15** | Notifications: **derive 7, persist 3.** The seven standing conditions (low stock, expiring batch, PO awaiting approval, overdue delivery, expiring document, expiring contract, maintenance due) are live queries — self-healing, no reconciliation, and the same queries drive the sidebar count pills. The three discrete events (new inquiry, batch quarantined, batch rejected) go in `notifications` + `notification_reads`. Visibility filtered by the viewer's permissions at read time. |
| **I16** | Schema gaps closed per C6. |
| **I17** | Files on the **filesystem, two trees, one Docker named volume**. `uploads/media/` served statically with content-hashed names; `uploads/documents/` streamed **only** through a Go endpoint behind `documents:read`, never statically reachable. QR images generated on demand, never stored. |
| **I18** | Deployment: single `docker-compose.yml` — `caddy`, `web`, `crm`, `api`, `db`, one-shot `migrate`. **Only `caddy` publishes ports.** Named volumes `pgdata`, `uploads`, `caddy_data`. Backup = `pg_dump` **plus** a tar of `uploads`; the restore test must cover **both**, or it passes while every certificate is gone. TLS and routing are environment-driven — see I24 and I25. |
| **I19** | Website port: **1:1 mechanical translation**. Markup → JSX, the 282-line script → component state, 13.7 KB CSS **verbatim**, assets extracted from the bundle. Hardcoded content arrays swapped for CMS/`items` fetches. **No Tailwind on the website** — that CSS is bespoke art, not a design system; there is no reuse to extract and every converted rule risks something already approved. |
| **I20** | The three tiers in §5 are a **build order**, not a cut list. Tier 0 first because it is the operational chain and carries the hardest correctness work, not because anything is at risk of being sacrificed. Every module ships complete. *(Superseded in intent: this began as a triage rule under a fixed deadline. With the deadline removed, the ordering survives and the degradation rule is withdrawn.)* |
| **I21** | `git init`, **no remote**. Trunk-based on `main`, annotated tag per completed slice. No hosted CI; the five gates run locally via `make check` and a pre-commit hook. *Accepted risk: no offsite copy of the repo, including the irreplaceable read-only prototypes.* |
| **I22** | Two seed commands: `seed:reference` (production-safe, idempotent — five products, packaging units, seed roles + permission matrix, locations, admin user, content page skeletons) and `seed:demo` (everything else), which **exits non-zero unless `APP_ENV != production`**. Demo batches with fabricated QC releases in `audit_log` would be a falsified regulatory record, not untidy data. A **staging stack** runs on the same box for staff training and for the deploy rehearsal in stage F. Empty states are a first-class deliverable. |
| **I23** | Localisation of status labels and error messages per C3. |
| **I24** | TLS is environment-driven: `TLS_MODE ∈ off \| internal \| auto`. `off` → plain HTTP, session cookie **not** `Secure`. `internal` → Caddy's own CA. `auto` → Let's Encrypt. The session cookie's `Secure` flag is derived from `TLS_MODE`, never hand-set. **The API refuses to boot when `APP_ENV=production` and `TLS_MODE != auto`** — going live insecure must be impossible, not merely discouraged. |
| **I25** | Until domains exist, the client tests over **plain HTTP on two ports**: website and CRM, one IP, no certificates, no browser warnings. *Accepted risk: the `Secure` cookie configuration is not exercised in the deployed environment.* Contained by I26 — an integration test asserts the cookie is `Secure; HttpOnly; SameSite=Lax` under `TLS_MODE=auto`, so the launch configuration is proven in the suite even though the test deployment never runs it. When domains land, `TLS_MODE=auto` and host-based routing on two subdomains; no application code changes. |
| **I26** | **Playwright E2E** covering the five ToR workflows through a real browser, in addition to the Go API-level tests. Also carries the cookie-configuration assertion described in I25 and the `TLS_MODE=auto` boot guard from I24. |
| **I27** | **CRM responsive pass is in scope.** `HANDOFF-CRM-CONTEXT.md:348` records that the prototype is desktop-only at ~1440px with a fixed two-column grid, against a ToR requirement (spec §4) for tablet and mobile. This was deferred for time; the time constraint is gone, the requirement is not. |
| **I28** | Scope additions stop there. **Финансы и бюджет** stays deferred — blocked on register question Q2, never on time (D2). **BOM / material reservation** stays out — `06-BUILD-PLAN.md:92` defines the production flow as *material issue*, and input→output traceability holds through `material_issue` movements referencing the MO, so the website's «Прослеживаемость» claim is satisfied without recipes. Both remain cheap to add on request; neither is assumed. |

---

## 3. Repository layout

Additions to `CLAUDE.md §2` marked ★.

```
/
├── Makefile                    ★ check · test · migrate · seed · gen · up · down
├── docker-compose.yml          ★ dev stack
├── docker-compose.prod.yml     ★ prod stack (caddy, web, crm, api, db, migrate)
├── Caddyfile                   ★
├── backend/
│   ├── cmd/api/                entrypoint
│   ├── cmd/seed/               ★ reference + demo, guarded
│   ├── internal/
│   │   ├── auth/               argon2id, sessions, lockout, middleware
│   │   ├── rbac/               Require(), permission resolution, startup route check
│   │   ├── audit/              ★ Record(ctx, tx, …)
│   │   ├── alerts/             ★ the 7 derived standing conditions + nav count pills
│   │   ├── http/
│   │   │   ├── common/         ★ envelope, list params, version guard, error mapping
│   │   │   └── …               per-module handlers
│   │   ├── domain/<module>/    business logic
│   │   └── db/                 sqlc-generated — never hand-edited
│   ├── migrations/             goose, sequential, immutable once applied
│   ├── queries/                hand-written SQL for sqlc
│   ├── testsupport/            ★ testcontainer + template-DB clone + audit assertion
│   └── tygo.yaml               ★
├── apps/crm/
│   ├── app/api/                BFF — the only caller of backend/
│   ├── app/styles/             ★ @theme tokens + verbatim layer ① primitives
│   └── messages/{ru,tg,en}.json ★
├── apps/web/
│   ├── app/[locale]/           ★ routed locales + hreflang
│   ├── public/                 ★ assets extracted from the prototype bundle
│   └── messages/{ru,tg,en}.json ★
└── packages/types/             tygo output — generated, never hand-edited
```

---

## 4. Phase 0 — foundations

Ordered by dependency. Nothing in Phase 1 starts until `make check` is green.

**Step 1 — unblock the humans first**

1. `git init`, `.gitignore`, `Makefile` skeleton.
2. **Extract the RU dictionaries and send them for translation immediately.** CRM chrome strings
   from the prototype's `T` object; website copy from the recovered source. This is external work
   with a long lead time (D10, `06-BUILD-PLAN.md:127`) that runs in parallel with everything else —
   the earlier it starts, the less it can ever become the blocker.
3. Extract the 4 image assets and Golos woff2 from the website bundle → `apps/web/public/`.
4. Extract prototype `:root` tokens → Tailwind `@theme`.
5. Monorepo scaffold: npm workspaces, `backend/`, `apps/crm`, `apps/web`, `packages/types`.
6. `docker-compose.yml` dev stack on `postgres:18`.

**Step 2 — the spine**

7. Base migration: universal conventions (`02-SCHEMA.md §1`), `users`, `sessions`, `roles`,
   `role_permissions`, `user_roles`, `audit_log`, `items`, `batches`. `batches` is pulled forward
   because QR generation depends on it and QR is needed *before* launch (D11).
8. sqlc wired; `queries/` established; first generated output committed.
9. Auth: argon2id, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, lockout, session cookie
   set by the BFF.
10. RBAC middleware + the **startup check that fails if any registered route lacks a permission
    declaration** (`04-RBAC.md:123`).

**Step 3 — the machinery eleven modules inherit**

11. `internal/http/common`: envelope, error mapping, pagination/sort/`q` parsing, version guard.
12. `internal/audit` + `internal/alerts`.
13. `tygo` wired; `packages/types` generating; staleness gate in `make check`.
14. Test harness: testcontainer, template-DB clone, **the mandatory audit assertion helper**.
15. `seed:reference`.

**Step 4 — the shell**

16. CRM shell in React: sidebar (252px), top bar (64px), permission-driven nav, global search,
    ТҶ/РУ/EN switcher, notification bell. Both chrome blocks exactly 64px so the divider is
    continuous across the seam (`CLAUDE.md §5`).
17. TanStack Query + BFF route pattern + auth cookie flow end to end.
18. Vitest + RTL + MSW harness; Playwright harness (I26).
19. **Screenshot drift gate run for the first time** (I13).
20. `make check` green. Tag `phase-0`.

---

## 5. Build order

Not a triage list. Every module ships complete — the ordering exists because dependencies and risk
run in this direction, so the hardest correctness work happens while attention is freshest and the
modules that consume it are built on something already proven.

| Tier | Modules | Why here |
|---|---|---|
| **0** | auth · RBAC + role management UI · `items` + QR export · `inventory` · `production` · `quality` · `procurement` · `inquiries` + public endpoints · CMS + published website · audit viewer · deploy + backup + restore test | The factory's operating chain: procurement → receipt → inventory → production → quarantine → QC release → shipment, plus the public site taking inquiries. Carries every hard correctness problem in the system. |
| **1** | `logistics` · `documents` · `dashboard` | Depend on Tier 0 data existing. `dashboard` aggregates everything above it, so it cannot be finished earlier. |
| **2** | `crm` · `hr` · `equipment` | Structurally similar list/detail/edit modules with no downstream dependents. By this point the pattern is mechanical. |

**Test depth is not a lever, in either direction.** `CLAUDE.md §7` sets full unit and integration
coverage and forbids reducing it. This plan does not treat it as negotiable.

---

## 6. Slice procedure

Every module after Товары follows this exactly. A slice is not done until every step passes.

1. Migration written, applied, matches `02-SCHEMA.md`.
2. `queries/<module>.sql`; `sqlc generate`; output committed.
3. Domain logic + unit tests.
4. HTTP handlers, `rbac.Require` on every route, `audit.Record` in every mutation.
5. Integration tests: happy path · validation failure · **403 without permission** · **401
   unauthenticated** · **audit row asserted**.
6. BFF route handlers.
7. `tygo` regenerated; `packages/types` current.
8. List view via the extracted engine + live search.
9. Detail view per `05-MODULES.md §2`.
10. Edit form with validation and `version_conflict` handling.
11. Component tests: loading · empty · error · populated.
12. Responsive behaviour verified at tablet and mobile widths (I27).
13. Seed data in `seed:demo`.
14. Screenshot drift gate (I13). `make check` green. Tag.

---

## 7. Delivery sequence

Dependency-ordered. Each stage completes before the next opens; no stage carries a date.

**A · Foundations** — Phase 0 per §4. RU dictionaries out for translation at the start, running in
parallel with everything below.

**B · Reference slice** — **Товары**, complete, reviewed line by line. QR generation and printer
export (D11). The engines are extracted from this working code before the next module (I2).

**C · The operational chain** — in this order, because each depends on the one before:

1. **Склад и запасы** — the ledger. Hardest correctness problem in the system.
2. **Производство** — depends on `items` and `batches`.
3. **Качество и безопасность** — depends on `batches`. Exhaustive transition matrix, reviewed line
   by line. The regulatory heart.
4. **Закупки и поставщики** — feeds inventory via `goods_receipt`.
5. **Интеграция с сайтом** — needed the moment the website exists.

**D · Remaining modules** — Логистика · Документы · Персонал · Оборудование и ТО · CRM и продажи ·
role management UI · audit log viewer · notifications.

**E · Website** — CMS models, workflow, media library, preview. `apps/web` port (I19). Public
endpoints, inquiry submission with reference numbers, rate limiting. Matomo, consent banner, legal
pages. **Панель управления last**, because it aggregates every module above it.

**F · Internal test** — the full suite green (`go test`, `sqlc diff`, `tygo` staleness, `vitest`,
`next build` ×2, **Playwright E2E across the five ToR workflows**), plus the responsive pass (I27)
and the screenshot drift gate (I13). Deploy to the **staging stack** and rehearse: migrations,
volumes, `seed:reference`, backup, **restore test**.

**G · Server deploy for client testing** — production compose stack on the Dushanbe box.
`TLS_MODE=off`, two ports, plain HTTP, no domain (I25). Client tests **both** systems over the IP.

**H · Feedback** — absorbed as ordinary slices. No compression, no triage.

**I · DNS and TLS** — domains registered and pointed at the box. `TLS_MODE=auto`, host-based
routing on two subdomains, Let's Encrypt. **No application code changes** — this is a Caddyfile and
an environment variable.

**J · Launch** — staff accounts and roles, Russian training materials, and the written note to QOIM
recording the offline limitation (D7, `01-DECISIONS.md:103`).

Stage F is an addition to `06-BUILD-PLAN.md §3`, which deploys straight to production. Rehearsing
on staging first means TLS, volumes and migration behaviour are discovered somewhere they do not
matter.

---

## 8. Client and external dependencies

Not developer work, and none of it is unblocked by writing more code. `06-BUILD-PLAN.md:138` says
send one consolidated request; these are its contents.

| Item | Blocks | Status |
|---|---|---|
| **Domain registration** — neither subdomain exists and registration has not started | Stage I. A public corporate website has no address without it, and `info@samari-kuhsor.tj` implies a domain that does not yet exist | ❗ **Not started.** `.tj` registration can require local documentation and is not same-day. Plausibly the longest external lead time in the project, and unlike translation it has no degraded fallback |
| **Server** — does the box exist, is there SSH, can it run Postgres 18 (C5), is Docker available | Stages F–G | ❗ Unconfirmed |
| Translations ru → tg, en + linguist review | Three-language launch (D10) | Sent at the start of Phase 0 |
| QR handoff to the wrapper printer | D11 — wrappers are ordered in advance against planned batch volume | Lead time may already have started |
| Product photography · retailer logos | Website imagery | Placeholders in place, degrade gracefully |
| Phone number · email confirmation | Contacts page | `+992 —` incomplete |
| News content and images · legal page copy · datasheet PDFs | Website content | Placeholders in place |
| **Q2 — accounting depth** | Финансы и бюджет entirely (D2) | Highest-priority open question |
| Q3 · Q4 barcodes · Q6 · Q9–Q12 · Q14 · Q15 | Various; `packaging_units.barcode` stays null until Q4 | Open |

**Two things worth stating in writing to QOIM**, per the pattern `06-BUILD-PLAN.md:139` establishes:

1. **Domain registration is now the long pole.** Everything else can be built, tested and deployed
   without it — the client can test both systems over an IP — but the website cannot be *published*
   to retail buyers on a bare IP that browsers label "Not secure".
2. **The offline limitation** (D7, `01-DECISIONS.md:103`): when Khorog's connection drops, data
   entry stops. There is no offline queue and no paper fallback. Quality records are the evidence
   trail behind the traceability claim, so a gap in them is a compliance gap. This is required of
   the developer in writing before launch.

---

## 9. Open — decided in-slice, not now

Deliberately unresolved. Recording them so their absence is a choice rather than an oversight.

- Sortable-field whitelist per resource (`03-API-CONTRACT.md:133`) — declared per module in its slice.
- Session idle timeout and absolute expiry values.
- Rate-limit thresholds for `POST /public/inquiries`.
- Matomo deployment shape and the inquiry retention period agreed in the clarification register.
- Exact CRM responsive breakpoints — the *decision to build it* is settled (I27); the breakpoint
  values are chosen when the shell is built.
- `packaging_units.barcode` stays null pending register Q4.
