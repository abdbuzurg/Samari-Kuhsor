# 08 — Remaining Work: T16–T38

Plan and test specification for everything after the Товары reference slice.

`TASKS.md` holds status. This file holds the *reasoning* for the remaining tasks:
what each one must not get wrong, and the tests that prove it didn't.

---

## 1. What is and is not implementable

| Tasks | State |
|---|---|
| T16–T34 | Buildable now. |
| **T35** staging rehearsal | Blocked: needs the Dushanbe server. Artefacts written, execution deferred. |
| **T36** server deploy | Blocked: same. |
| **T37** client feedback | Blocked: needs the client. |
| **T38** DNS, TLS, launch | Blocked: domains not registered (see `07-IMPLEMENTATION-PLAN.md §8`). |

For the blocked four, the deliverable is the *artefact* — compose file, Caddyfile,
backup and restore scripts, a written runbook — verified as far as it can be
without the box.

---

## 2. Schema strategy

Migrations are sequential and immutable once applied (`02-SCHEMA.md §11`). The
remaining tables arrive in three migrations grouped by when they are needed, not
one per module: a module's tables are useless without its neighbours' foreign
keys, and twelve migrations that must be applied in order are twelve chances to
apply them out of order.

- **00002** — the operating chain: `locations`, `stock_movements`,
  `stock_balances` (view), `manufacturing_orders`, `production_entries`,
  `quality_tests`, `batch_status_events`.
- **00003** — commerce and administration: suppliers, purchase orders, goods
  receipts, shipments, vehicles, drivers, customers, contacts, leads, deals,
  `deal_stage_events`, sales orders, inquiries, tasks, departments, positions,
  employees, assets, maintenance events, documents, document versions.
- **00004** — CMS and notifications: content pages, blocks, translations, news,
  media, workflow events, `notifications`, `notification_reads`.

---

## 3. T16 · Склад и запасы — the ledger

**The hardest correctness problem in the system.** Everything else in this file is
ordinary CRUD by comparison.

Non-negotiables (`CLAUDE.md §4.2`, `02-SCHEMA.md §5`, `07-IMPLEMENTATION-PLAN.md`
I5/I6):

- No balance column exists. `stock_balances` is a **plain view** (I5).
- Corrections are **compensating entries**. The original row is never updated or
  tombstoned.
- A transfer is **two rows** sharing a `ref_id`, netting to zero.
- Any transaction posting a **negative** delta takes
  `pg_advisory_xact_lock(item, batch, location)` and re-reads inside the lock.
- Going negative is refused `422` — **except `reason = 'adjustment'`**, which must
  be able to, because it is the correction mechanism and blocking it would make
  errors uncorrectable.
- **No endpoint anywhere accepts an absolute quantity.** The UI must never offer
  "set stock to X" (`05-MODULES.md:112`).

### Tests

| Test | Proves |
|---|---|
| Balance equals the sum of deltas, over hundreds of movements | The view is the truth |
| Transfer posts two rows, shares `ref_id`, nets to zero | Transfers cannot leak stock |
| Correction is a new row; the original is byte-identical afterwards | Append-only holds |
| Two concurrent issues of 80 against 100 → one succeeds, one 422 | The advisory lock works |
| Same, but `adjustment` → both succeed, balance goes negative | The exemption works |
| Every reason code round-trips and is rejected if unknown | Enum integrity |
| No request body field anywhere sets an absolute quantity | Asserted by reflection over the DTOs |
| Tombstoned movements are excluded from the balance | Deletion semantics |
| Balance is per `(item, batch, location)`, not aggregated wrongly | Grouping |
| Low-stock and expiry alerts derive from the view + `min_qty` | Alerts are self-healing |

---

## 4. T17 · Производство

- MO ↔ batch is **1:1**.
- Actual output, yield and downtime are **sums over `production_entries`**, never
  columns.
- Completing an order posts a `production_output` movement into a **quarantine**
  location and moves the batch to `quarantine`.
- Completion does **not** make the batch sellable. Only `quality` can.

### Tests
Yield is computed not stored · completion posts to quarantine, never to finished
goods · completing does not set `released` · `production_entries` are append-only ·
an order cannot be completed twice · material issue posts negative movements.

---

## 5. T18 · Качество и безопасность — the regulatory heart

**Highest test coverage in the codebase** (`05-MODULES.md:138`). Reviewed line by
line.

Transition rules (`02-SCHEMA.md §7`):

```
in_production → quarantine   automatic on production completion
quarantine    → released     requires quality:approve
quarantine    → rejected     requires quality:approve
released      → rejected     recall — requires quality:approve, reason mandatory
rejected      → (terminal)
```

### Tests

**The full matrix**: every from/to pair — 4 × 4 = 16 — legal and illegal, each
with and without `quality:approve`. 32 cases, exhaustive, no sampling.

Plus: release writes an immutable `batch_status_events` row **and** an audit entry
naming the deciding user · recall requires a reason and is refused without one ·
`rejected` is terminal from every direction · **a sales order line refuses a
non-`released` batch** · **a shipment line refuses a non-`released` batch**, both
enforced in the domain and proven server-side, not in the UI.

---

## 6. T19 · Закупки и поставщики

`procurement:approve` gates exit from `approval`. Goods receipt posts
`goods_receipt` movements — this is how raw material enters inventory.

**Tests:** approval without the permission is 403 · receipt quantities match the
posted movements exactly · receiving against a closed PO is refused · partial
receipt leaves the PO open · over-receipt is refused or flagged.

---

## 7. T20 · Интеграция с сайтом + T30 · public endpoints

Reference prefixes: `WR-` wholesale · `CF-` contact · `DA-` distributor ·
`CP-` complaint · `JB-` job. Every submission returns its number — a ToR
requirement.

**Tests:** each type produces its prefix · numbers are unique under concurrent
submission · a `CP-` complaint must link to a batch · conversion to a lead carries
the reference across · `POST /public/inquiries` needs no session · rate limiting
by IP · the public surface cannot reach any CRM endpoint · **personal data never
appears on a public endpoint.**

---

## 8. T21–T25 · Логистика, Документы, Персонал, Оборудование, CRM

Structurally similar; by now the pattern is mechanical. Each still gets the full
permission matrix and audit assertions.

Module-specific rules that are **not** mechanical:

- **Логистика**: loading a shipment line with a non-`released` batch is refused
  server-side.
- **Документы**: superseded versions are retained, never deleted ·
  `documents:approve` gates `approval → active` · files unreachable by any static
  path, requiring `documents:read` (I17).
- **Персонал**: personal data is unreachable through every public endpoint,
  asserted by test · contract expiry warns at 30 days · no payroll.
- **CRM**: confirming a sales order posts `sale` movements and refuses
  non-`released` batches · deal stage history is append-only.

---

## 9. T26 · Role management and audit viewer

**Guardrail that must be enforced server-side:** the last holder of `admin:manage`
cannot be deactivated or stripped of it. Without it, one careless edit locks
everybody out of a system with no other way in.

**Tests:** the last admin cannot be demoted or deactivated · permission changes
take effect on the affected user's **next request** (nothing cached beyond the
request) · every role change writes to `audit_log` · audit rows are not editable
or deletable through any route · system roles are editable but not deletable.

---

## 10. T27 · Notifications

Per I15: **derive 7, persist 3.**

**Tests:** a resolved condition disappears with no retraction logic · users never
see a notification for a resource they cannot `read` · the count pills and the
bell come from the same service and cannot disagree · `Pending()` is empty once
all seven queries are attached.

---

## 11. T28 · CMS

Ladder: `draft → technical_review → language_review → approved → published`.

**Tests:** every illegal transition refused · `approved`/`published` require
`cms:approve` · the public API returns only `published` · the CRM can preview any
state · every transition writes `content_workflow_events` **and** an audit entry.

---

## 12. T29 · Website port

1:1 mechanical translation of the recovered source (I19). CSS verbatim.

**Tests / checks:** belt roll-in, batch paging, three-stage map draw, marquee and
replay-on-return all behave as `PROJECT-CONTEXT-WEBSITE.md §7` describes ·
`prefers-reduced-motion` degrades to static placement plus fades · **the assembly
line stays horizontal and swipeable on mobile — it must never become a vertical
list** · only the five real products appear · `уточняется` posture preserved.

---

## 13. T33 · Панель управления

Built last; it aggregates everything.

**Tests:** the Дебиторка card is **hidden** (no receivables without finance) ·
Выручка comes from confirmed sales orders only · **empty states render rather than
zeros or the prototype's sample numbers** (`05-MODULES.md:70`) · the period switch
re-plots the chart and the revenue KPI only.

---

## 14. T34 · Full pass

`make check` plus Playwright across the five ToR workflows (I26):

1. Procurement — request → approval → PO → delivery → inspection → receipt
2. Production — plan → material issue → batch → output into quarantine
3. Quality — tests recorded → release or reject → stock becomes sellable
4. Sales — inquiry → lead → order → stock check → shipment of a released batch
5. Complaint — complaint with reference → batch traceability → investigation → closure

These are the acceptance criteria. They are written as **Go API-level integration
tests** as well, so the business rules are proven without a browser.
