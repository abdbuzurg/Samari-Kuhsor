# 06 — Build Plan

How the work is sequenced between 17 August and 9 September 2026. Solo developer.

---

## 1. Method: vertical slices

Do **not** build all migrations, then all handlers, then all UI. Build one module completely
through every layer, review it hard, then replicate the pattern.

The first slice — **Товары и цены** — is the reference implementation. Everything after it is
copy-and-adapt. Time spent making that slice right is repaid ten times.

### Slice checklist

A module is done when every box is ticked:

- [ ] Migration written, applied, matching `02-SCHEMA.md`
- [ ] Queries in `queries/<module>.sql`, `sqlc generate` run, output committed
- [ ] Domain logic in `internal/domain/<module>/` with unit tests
- [ ] HTTP handlers with `rbac.Require` on every route
- [ ] Integration tests: happy path, validation failure, 403 without permission, 401 unauthenticated
- [ ] Audit log written on every mutation
- [ ] BFF route handlers in `apps/crm/app/api/`
- [ ] Types in `packages/types`
- [ ] List view matching the prototype, with live search
- [ ] Detail view following the shared pattern in `05-MODULES.md` §2
- [ ] Edit form with validation and version conflict handling
- [ ] Component tests: loading, empty, error, populated
- [ ] Seed data demonstrating the module
- [ ] Test suite green before the next slice opens

---

## 2. Order of work

### Phase 0 — Foundations (before any module)

1. Monorepo scaffold; Go module; two Next.js apps; `packages/types`; CI running the test suite.
2. Postgres, goose, sqlc wired. Base migration: conventions from `02-SCHEMA.md` §1, `users`,
   `sessions`, `roles`, `role_permissions`, `user_roles`, `audit_log`, plus `items` and `batches` —
   `batches` is pulled forward because QR generation depends on it and QR is needed before launch.
3. Auth: login, logout, `/auth/me`, argon2id, session cookie via BFF, lockout.
4. RBAC middleware plus the startup check that fails if any route lacks a permission declaration.
5. API skeleton: envelope, error format, pagination, sorting, search helpers. Get these right once —
   eleven modules inherit them.
6. Seed command: five products, seed roles, one admin user, locations.
7. CRM shell ported to React: sidebar, top bar, permission-driven nav, search, language switcher.

Foundations are not glamorous and they are where the leverage is. Do not shortcut them to reach
a demoable module sooner.

### Phase 1 — Reference slice

8. **Товары и цены**, complete, including QR generation and export (D11 — needed before launch).

Review this slice properly before continuing. Every mistake in it multiplies by eleven.

### Phase 2 — Operational modules, in dependency order

9. **Склад и запасы** — the ledger. Second because it is the hardest correctness problem.
10. **Производство** — depends on items and batches.
11. **Качество и безопасность** — depends on batches; exhaustive transition tests.
12. **Интеграция с сайтом** — needed the moment the website is live.
13. **Закупки и поставщики** — feeds inventory.

### Phase 3 — Remaining modules

14. Логистика · Персонал · Оборудование и ТО · Документы · CRM и продажи.
    These are structurally similar list/detail/edit modules; by now the pattern is mechanical.
15. Role management UI and the audit log viewer (`04-RBAC.md` §6).
16. Notifications with real triggers.

### Phase 4 — Website

18. CMS models, workflow, media library, preview.
19. `apps/web` ported to Next.js. Rebuild the animations carefully: staggered belt roll-in, the
    three-stage map border draw, marquee, replay-on-view-return, reduced-motion fallbacks. These are
    the client's favourite parts of the site — regressions here will be noticed immediately.
20. Public endpoints, inquiry submission with reference numbers, rate limiting.
21. Matomo self-hosted, consent banner blocking analytics until accepted, inquiry retention policy.
    All three were agreed in the clarification register and exist nowhere yet.
22. Legal pages: privacy policy, terms, cookies.
23. **Панель управления** last — it aggregates every module above, so it cannot be finished earlier.

### Phase 5 — Launch

24. Deploy: TLS, both subdomains, backups with a **restore test**, log rotation, monitoring.
25. Staff accounts and roles for QOIM.
26. Smoke test of the five end-to-end workflows from the ToR:
    1. **Procurement** — request → approval → PO → delivery → inspection → receipt into stock.
    2. **Production** — plan → material issue → batch → output into quarantine.
    3. **Quality** — tests recorded → release or reject → stock becomes sellable.
    4. **Sales** — inquiry → lead → order → stock check → shipment of a released batch.
    5. **Complaint** — complaint with reference number → batch traceability → investigation → closure.
27. Training materials in Russian, and a short written note to QOIM recording the offline
    limitation (D7).

---

## 3. Calendar

| Dates | Target |
|---|---|
| 17–20 Aug | Phase 0 foundations |
| 21–23 Aug | Товары slice complete and reviewed; QR export working |
| 24–27 Aug | Склад, Производство |
| 28–31 Aug | Качество, Интеграция с сайтом, Закупки |
| 1–3 Sep | Remaining five modules, role management, audit viewer, notifications |
| 4–5 Sep | CMS, website port |
| 6–7 Sep | Public endpoints, Matomo, consent, legal pages, Панель управления |
| 8 Sep | Deploy, restore test, smoke test, accounts, training |
| **9 Sep** | **Launch** |

This calendar has no slack. Any slippage comes out of Phase 3 first — those modules describe
activity that has not started yet on opening day, so they are the least damaging to compress.

---

## 4. Parallel track — client dependencies

None of this is developer work, and all of it is on the critical path. Chase it from day one.

| Item | Needed by | Blocks |
|---|---|---|
| **Translations ru → tg, en** + linguist review | ~1 Sep | All three languages live (D10). Longest lead time in the project — send the Russian copy this week. |
| **QR handoff to the wrapper printer** | Immediately | Wrappers are ordered in advance; the lead time may already have started (D11). |
| Product photography | ~1 Sep | Every product image is currently a placeholder |
| Retailer logos | ~1 Sep | Marquee uses placeholder names |
| Phone number, email confirmation | ~1 Sep | Contacts page |
| News content and images | ~3 Sep | News section |
| Legal page copy | ~3 Sep | Footer links |
| Technical datasheet PDFs | ~3 Sep | Product detail downloads |
| **Q2 — accounting depth** | ASAP | Финансы и бюджет entirely |
| Q3, Q4 barcodes, Q6, Q9–Q12, Q14, Q15 | ASAP | Various |

Send a single consolidated request to QOIM with dates against each item. Anything not received by
its date should be raised in writing rather than absorbed silently.

---

## 5. Risk register

| Risk | Likelihood | Impact | Response |
|---|---|---|---|
| Solo capacity — 650–750h of work against ~280h available | High | Severe | Accepted (D1). Compress Phase 3 first. |
| Client content arrives late or not at all | High | High | Chase now with dates; placeholder fallbacks that degrade gracefully |
| Translations miss linguist review | High | Medium | **Escalate, do not decide unilaterally.** D10 commits to all three languages at launch. If translations slip, raise it with QOIM in writing and have them choose between delaying the switcher and shipping unreviewed copy. |
| Khorog connection drops at launch | Medium | High | Accepted (D7). Put the limitation in writing to QOIM. |
| Ledger arithmetic wrong | Medium | Severe | Exhaustive tests; movements never edited, only compensated |
| Batch released without QC | Low | Severe | Server-side enforcement plus full transition test matrix |
| Staff untrained on opening day | High | Medium | Russian training materials; seed roles pre-configured |
| Q2 answered late and finance is in scope after all | Medium | Severe | Module already quarantined; treat as a new phase with its own timeline |

---

## 6. Working with Claude Code

- Start every session by having it read `CLAUDE.md` and the relevant `docs/` file. It should not
  design a schema, invent a SKU, or choose a status colour — those decisions are written down.
- Work **one slice per session**. Long sessions spanning several modules produce drift.
- Ask for the tests in the same change as the code, never as a follow-up.
- If it proposes reopening a decision in `01-DECISIONS.md`, that is a signal it has not read it.
- Review the Товары slice line by line. Skim everything after it — but skim it against the
  checklist in §1, not against a feeling.
