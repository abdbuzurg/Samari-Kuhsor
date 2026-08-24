# 09 — Recovery Plan: R00–R19

Written 24 August 2026, after `TASKS.md` was re-derived from the code rather than
trusted. Ordering is completion-driven, not date-driven (`CLAUDE.md` §1).

`TASKS.md` holds status. `08-REMAINING-PLAN.md` holds the reasoning for T16–T38.
**This file supersedes both for everything still outstanding.**

---

## 1. Why this file exists

`TASKS.md` reported T01–T33 `done`. An audit of the code found that thirteen of
those tasks passed gates that never touched the frontend.

The defect is in how the gates were written, not in the work. Every gate was
phrased against the domain layer:

> T25 · **Done when:** confirming a sales order posts `sale` movements and
> refuses non-`released` batches.

That is a true statement about Go. It says nothing about whether a human can
confirm a sales order. They cannot: `useConfirmSalesOrder` is defined in
`apps/crm/lib/operations.ts`, wired through the BFF to a working endpoint, and
**called by no component**.

### The rule that replaces it

> **A gate is only valid if it names a role, an action, and a browser.**
>
> *"A user holding `quality:approve` can release a quarantined batch from
> `/quality/{id}` and the batch becomes sellable"* — valid.
>
> *"Releasing a batch requires `quality:approve`"* — invalid. That is a domain
> assertion and the domain already has 830 tests.

Every gate in this file is written that way. No R-task closes on a Go test alone.

---

## 2. The true baseline

### Backend — substantially complete

The Go API is not the problem and mostly needs no work. Append-only ledger with
advisory-lock oversell guards, a 32-case batch transition matrix, RBAC verified
against the router at boot, audit trail on every mutating domain, per-type
inquiry sequences, the public surface. The T34 live run drove the full
production → quality → sale chain end to end. It was driven by `curl`.

### Frontend — a read-only browser over that API

| Module | List | Detail | Create | Actions | Status |
|---|---|---|---|---|---|
| Товары | ✅ | ✅ | ✅ | ✅ | **complete** |
| Контент (CMS) | ✅ | — | ✅ | ✅ | **complete** |
| Администрирование | ✅ | — | ✅ | ✅ | **complete** |
| Журнал действий | ✅ | n/a | n/a | n/a | **complete** |
| Склад и запасы | ✅ | ❌ | ❌ | ❌ | read-only |
| Производство | ✅ | ❌ | ❌ | ❌ | read-only |
| Качество | ✅ | ❌ | ❌ | ❌ | read-only |
| Закупки | ✅ | ❌ | ❌ | ❌ | read-only |
| Логистика | ✅ | ❌ | ❌ | ❌ | read-only |
| Персонал | ✅ | ❌ | ❌ | ❌ | read-only |
| Оборудование | ✅ | ❌ | ❌ | ❌ | read-only |
| Документы | ✅ | ❌ | ❌ | ❌ | read-only |
| Обращения | ✅ | ❌ | ❌ | ❌ | read-only |
| CRM и продажи | ✅ | ❌ | ❌ | ❌ | read-only, **5 of 6 entities unbuilt** |

Ten list views pass `rowHref` to `ListView`, which renders rows as real links
(`ListView.tsx:249`). Only `/items/{id}` exists. **Ten modules 404 on row click.**

### Fourteen orphaned mutation hooks

Defined, typed, wired to working endpoints, called by nothing:

```
useRecordEntry             useConfirmSalesOrder       useTransitionDocument
useCompleteOrder           useLoadShipment            useNewsTranslations
useBatchDetail             useConvertInquiry          useSaveNewsTranslation
useSuppliers               useRecordMaintenance       useMediaLibrary
useTransitionPurchaseOrder useReceivePurchaseOrder
```

And two that were never written at all: **there is no `useTransitionBatch` and no
`useRecordQualityTest`.** Batch release — the operation the entire quality chain
exists to gate, and ToR §8 acceptance condition 5 — has no client code of any kind.

### CRM и продажи is a different module from the one specified

`05-MODULES.md:179` specifies KPIs *Новые лиды · Открытые сделки · Конверсия ·
Просроченные задачи*, columns *Клиент · Тип · Регион · Статус · Сумма · Менеджер*,
a five-stage pipeline, and a detail view of customer · contacts · deals · linked
inquiries · orders · activity.

What ships is a sales-order table. **No specified column matches.** `customers`,
`contacts`, `leads`, `deals`, `deal_stage_events` and `tasks` have no INSERT, no
list query, no route and no UI. `CreateCustomer` and `CreateLead` are reachable
only as a side effect of inquiry conversion, producing records no screen can open.

Consequence in the shipped product: the dashboard's **Воронка продаж** reads
`deals` (`inventory.sql:326`), which nothing writes. The funnel is structurally,
permanently empty.

### Seed

`cmd/seed/main.go:66` — *"demo seed is not built yet."* The reference seed loads
items, translations, locations, packaging units, roles, permissions and users.
Nothing else. A client logging in to a deployed instance sees empty tables in
every operational module.

### ToR §8 acceptance conditions

| # | Condition | Now | After this plan |
|---|---|---|---|
| 1 | Website inquiries create CRM leads automatically | ❌ | R07 |
| 2 | Orders trackable inquiry → delivery → payment | ❌ | R06, R13 (payment out of scope) |
| 3 | Products traceable to raw-material batches | ❌ | R03 |
| 4 | Warehouse balances by item, batch, expiry | ✅ | holds |
| 5 | Quality staff can quarantine and release | ❌ | **R03** |
| 6 | Budgets planned / committed / actual | ❌ | **out of scope — written variation owed to QOIM** |
| 7 | Reports exportable · permissions enforced · backups restorable | ⚠️ | R15/R16 · holds · T35 |

Of ToR §5's four in-scope end-to-end workflows, **zero are executable through the
browser today.**

---

## 3. What is being built, and in what order

Both branches: breadth (every module clickable) **and** depth (the operational
chain fully writable). The sequencing rule is what makes that survivable:

> **Order tasks so that if time runs out, what is unfinished is the least
> damaging thing — not whatever happened to be in hand.**

On 9 September the plant has no customers, no deals and no document history. It
does have juice coming off a line that legally cannot ship until someone releases
the batch. So the operational chain is built first and the registers last, even
though the registers are cheaper and would make the burndown look better.

### Tiers

- **Tier 0 · Scaffold** (R00–R02). Build the shared machinery once. `DetailView`
  already exists and is proven in Товары; `WorkflowActions` exists but is
  hardcoded to the CMS. Generalising these is what makes ten detail views cheap.
- **Tier 1 · The operational chain** (R03–R07). Качество, Производство, Склад,
  Логистика, Обращения. Full write paths. **These stop the factory if absent.**
- **Tier 2 · Registers** (R08–R11). Закупки, Документы, Оборудование, Персонал.
  Detail view plus the actions each already has an endpoint for.
- **Tier 3 · CRM** (R12–R13). The five unbuilt entities, back to front.
- **Tier 4 · Cross-cutting** (R14–R19). Demo seed, exports, responsive, E2E.

---

## 3a. Progress

| Task | Status | Evidence |
|---|---|---|
| R00 · Stop lying to the user | **done** | 10 `rowHref` removed; only `/items/{id}` links. `TASKS.md` re-baselined |
| R01 · Generalise the detail scaffold | **done** | `ActivityPanel`, `RelatedTable`, `DetailShell`, generalised `WorkflowActions`; +17 tests (201 total) |
| R02 · Five missing GET-by-id endpoints | **done** | 5 Go routes + 5 BFF routes + 5 hooks; `detail_routes_test.go`; full Go suite green |
| R03 · Качество — the release screen | **done** | `/quality/{id}`; `useTransitionBatch` via generalised `useTransition`, `useRecordQualityTest` written; +13 tests (214 total) |
| R04 · Производство | **done** | `/production/{id}`; shift entries + completion; orphaned `useRecordEntry`/`useCompleteOrder` wired |
| R05 · Склад | **done** | `/inventory/ledger`; delta-only movement and transfer forms; +11 tests |
| R06 · Логистика | **done** | `/logistics/{id}`; loading list restricted to released batches |
| R07 · Обращения | **done** | `/inquiries/{id}`; conversion wired; complaint → batch link |
| R08 · Закупки | **done** | `/procurement/{id}`; PO ladder + goods receipt |
| R09 · Документы | **done** | `/documents/{id}`; approval ladder |
| R10 · Оборудование | **done** | `/equipment/{id}`; service history + recording |
| R11 · Персонал | **done** | `/hr/{id}`; edit form with optimistic concurrency |
| R12 · CRM backend | **done** | `crm` domain: customers, contacts, leads, deals, stage events, tasks; 13 routes; exhaustive stage matrix |
| R13 · CRM frontend | **done** | Register to spec, pipeline board, deal detail, customer detail, sales orders moved to `/crm/orders` |
| R14 · Demo seed | **done** | `seed demo`; every module populated; refuses to run twice or in production |
| R15 · Export | **done** | One CSV exporter over the guarded list handlers; 14 collections; buttons on every register |
| R19 · Notice to QOIM | **drafted** | `docs/10-CLIENT-NOTICE.md` — **not complete until sent and acknowledged** |
| R20 · Create forms *(added)* | **done** | Ten `useCreate*` hooks were defined and called by nothing — the founding defect, one layer down. Now: batch, MO, trip, employee, asset, document, PO (with lines), sales order (with released-batch lines), customer, supplier, task |
| R21 · CMS gaps *(added)* | **done** | News translation editor and per-locale media alt text. Publishing needs all three languages (D10) and there was no screen that could supply them — a post could be created and never published |
| R16 · Printable documents | **done** | Batch certificate · purchase order · delivery note as print routes (see below) |
| R17 · Responsive pass | **done** | Sidebar → drawer below `lg`; detail and form grids collapse; no horizontal body scroll at 390px |
| R18 · Playwright E2E | **done** | 22 specs, **all passing against a live stack** — Postgres + Go + both Next apps |

### R16 — why print routes rather than server-side PDF

The gate was that Cyrillic and Tajik `ҳ ҷ ӯ` render correctly. Every Go PDF
library needs an embedded TTF covering those glyphs; this project's font ships as
a subset woff2 loaded through next/font. Converting and re-embedding it is a
glyph-coverage gamble whose failure mode is silently printing mojibake onto a
regulatory document.

A print route inherits the fonts the application already loads and already
proved. «Печать → Сохранить как PDF» produces a correct, selectable PDF with no
new dependency and no second rendering path to keep in step with the screen. The
cost is that it cannot be generated unattended, which nothing in the first
release needs. If batch certificates ever need to be produced in bulk without a
human, that is a separate task with a separate font decision.

---

**Defects found while building, all now fixed.** The last four were found ONLY by
driving a browser against a live stack — which is the point:

1. **The R02 handlers double-wrapped their envelopes** — `{"data":{"data":…}}`,
   the exact defect T34 found 27 times. Caught by the new detail tests.
2. **The envelope guard cannot see detail routes.**
   `TestEverySuccessfulResponseIsASingleDataEnvelope` returns early on any
   non-200, and every `/{id}` case in `everyGuardedRoute` uses `zeroUUID`, which
   404s. So the guard has never checked a single detail response. Documented in
   place; detail routes are covered by `detail_routes_test.go`, which creates a
   real record first.

---

## 4. Task register

Estimates are solo hours. Each gate names a role, an action and a browser.

### Tier 0 — Scaffold

#### R00 · Stop lying to the user — `1h`
Remove `rowHref` from every module without a detail route. Rewrite the T16–T33
status lines in `TASKS.md` to match section 2 of this file.

**Done when:** clicking a row in every module either opens a detail view or does
nothing; no route in the CRM renders a 404 reachable from a link. `TASKS.md`
contains no `done` that this audit contradicts.

> Do this first and do it today. It is five minutes of code and it is the
> difference between a client finding a gap and a client finding a broken link.

#### R01 · Generalise the detail scaffold — `8h`
- `WorkflowActions` currently takes `kind: 'pages' | 'news'` and calls
  `useCMSTransition` directly. Generalise to accept a mutation function and a
  label map. Keep the `allowed_transitions`-from-server rule — the whole point of
  that component is that the UI never recomputes the matrix.
- New `ActivityPanel` reading `/audit?resource=&resource_id=`. Both query
  parameters already exist (`audit.sql:32-33`) and `ListAuditForResource` is
  already written. This satisfies `05-MODULES.md §2`'s activity row for every
  module at once.
- New `RelatedTable` for the "related records" band.
- A detail-page skeleton with loading, error and not-found states, so ten pages
  do not each invent their own.

**Done when:** a user can open `/items/{id}` and see an activity panel listing
that item's own audit rows, newest first; the CMS ladder still works through the
generalised `WorkflowActions` with no behaviour change; all three components have
loading/empty/error/populated tests per `CLAUDE.md §7`.

#### R02 · The five missing GET-by-id endpoints — `6h`
No detail view can exist without one. Missing for `employees`, `assets`,
`documents`, `inquiries`, `suppliers`.

**The SQL already exists** — `GetEmployee`, `GetAsset`, `GetDocument`,
`GetInquiry`, `GetSupplier` are all in `queries/`. This is handler + route + BFF
+ type only.

**Done when:** each returns `{data}` for a real id, `404` for an unknown one and
`403` without the module's `read`; each has an integration test covering all
three; `packages/types` regenerated and not stale.

### Tier 1 — The operational chain

#### R03 · Качество — the release screen — `12h` ⚠️ **highest priority in the plan**
ToR §8 condition 5 and the regulatory heart of the system. Currently has no
client code at all.

- Write `useTransitionBatch` and `useRecordQualityTest` — neither exists.
- `/quality/{id}`: batch header · all tests with results and inspectors · status
  history with who decided and when · where the stock is now (`05-MODULES.md:149`).
- Release / quarantine / reject actions via the generalised `WorkflowActions`,
  driven by the server's `AllowedFrom`.
- Recall (`released → rejected`) must collect a reason — the domain already
  refuses without one.

**Done when:** a user holding `quality:approve` can release a quarantined batch
from the browser and it becomes sellable; a user holding only `quality:manage`
sees no release button **and** gets a refusal if they forge the request; a recall
without a reason is refused with the message rendered; the detail view shows the
batch's full test and decision history.

#### R04 · Производство — entries and completion — `10h`
`/production/{id}`: order header · batch · append-only shift entries · yield.
Wire the orphaned `useRecordEntry` and `useCompleteOrder`.

**Done when:** a production user can record a shift entry (good / scrap /
downtime) and see it appended; completing an order posts output to quarantine and
the batch appears in Качество as quarantined; completing twice is refused with
the message rendered; yield shows `уточняется`, not `0`, before anything has run.

#### R05 · Склад — movements and transfers — `12h`
`/inventory/{item}/{batch}/{location}`: the position's full movement ledger with
running balance (`05-MODULES.md:112`).

**The hard constraint** (`05-MODULES.md:112`, `08-REMAINING-PLAN.md §3`): no form
may offer an absolute quantity. Приёмка / перемещение / списание / корректировка
are all **deltas**. There is no "set stock to X" and there must never be one.

**Done when:** a warehouse user can post a receipt, an issue, a transfer and a
correction from the browser; a transfer renders as two rows sharing a `ref_id`
netting to zero; an issue that would go negative is refused with the message
shown; the same as an `adjustment` succeeds; **no input in the CRM accepts an
absolute quantity, asserted by a test over the rendered forms.**

#### R06 · Логистика — trips and loading — `10h`
`/logistics/{id}`: trip header · driver · vehicle · loading list · status. Wire
`useLoadShipment`. Add trip creation.

**Done when:** a logistics user can create a trip, load a released batch onto it
and advance it to delivered; loading a quarantined batch is refused server-side
with the refusal rendered; the ToR sales workflow is walkable from sales order to
delivered trip.

#### R07 · Обращения — conversion — `6h`
ToR §8 condition 1. `/inquiries/{id}`: submission detail · reference number ·
named batch for complaints · convert action. Wire `useConvertInquiry`.

**Done when:** a sales user can convert an inquiry from the browser; the created
customer and lead are reachable from the resulting screen (depends on R13 —
until then, link to the sales order); converting twice is refused with the
message rendered; a `CP-` complaint links through to its batch's traceability
view from R03.

### Tier 2 — Registers

#### R08 · Закупки — `12h`
`/procurement/{id}`: PO header · supplier · lines · receipt history · linked stock
movements · activity (`05-MODULES.md:199`). Wire `useTransitionPurchaseOrder`,
`useReceivePurchaseOrder`, `useSuppliers`. Add PO creation and a supplier register.

**Done when:** a procurement user can raise a PO, send it for approval, have a
holder of `procurement:approve` approve it, and receive goods against it, all from
the browser; the receipt appears in Склад as `goods_receipt` movements; approval
without the permission shows no button and is refused if forged.

#### R09 · Документы — `8h`
`/documents/{id}`: versions retained, approval ladder, file access behind
`documents:read` (I17). Wire `useTransitionDocument`.

**Done when:** a documents user can upload a document, send it for approval, and a
holder of `documents:approve` can activate it; a superseded version is still
retrievable; the file is unreachable by any static path without `documents:read`.

#### R10 · Оборудование — `7h`
`/equipment/{id}`: asset header · service history · next due · warranty. Wire
`useRecordMaintenance`.

**Done when:** an equipment user can record maintenance and the asset's
`maintenance_due` flag clears; recording maintenance on a `broken` asset does
**not** clear `broken`; the next-due date updates on the register.

#### R11 · Персонал — `7h`
`/hr/{id}`: employee file · position · contract dates · shift. Create and edit.

**Done when:** an HR user can create and edit an employee from the browser; a
contract expiring inside 30 days renders `warn`; **the detail payload is
unreachable through every public endpoint, re-asserted by test** — the T23 gate
was the one thing that slice got right and it must survive a new route.

### Tier 3 — CRM и продажи

#### R12 · CRM backend — `16h`
Queries, domain and handlers for `customers`, `contacts`, `leads`, `deals`,
`deal_stage_events`, `tasks`.

**No migration is needed.** All six tables exist in `00003_commerce.sql`, are
correctly shaped, and `deals.stage` already carries the exact five-stage
enum the spec's pipeline needs (`new`, `negotiation`, `quoted`, `won`, `lost`).

Deal stage changes write `deal_stage_events` the way batch transitions write
`batch_status_events` — the pattern is already in `quality/`, follow it.

**Done when:** every entity has list, get, create and update behind `crm:read` /
`crm:manage`; moving a deal's stage writes an immutable `deal_stage_events` row
naming the user; every mutation writes `audit_log`; integration tests cover happy
path, validation failure and permission denial per `CLAUDE.md §7`.

#### R13 · CRM frontend — `14h`
Rebuild `/crm` to the specification in `05-MODULES.md:179`, which the current page
does not implement.

- KPIs: Новые лиды · Открытые сделки · Конверсия · Просроченные задачи
- Columns: Клиент · Тип · Регион · Статус · Сумма · Менеджер
- Pipeline: Новый лид → Переговоры → КП отправлено → Выиграно / Проиграно
- `/crm/{id}`: customer header · contacts · deals with stage history · linked
  inquiries · orders · activity
- Sales orders move to their own view under the module.

**Done when:** a sales user can create a customer, add a contact, open a deal,
move it through all five stages and see the stage history; **the dashboard's
Воронка продаж renders real bars** rather than the permanent empty state; the four
specified KPIs compute from real rows; converting an inquiry (R07) lands on a
customer screen that opens.

### Tier 4 — Cross-cutting

#### R14 · Demo seed — `8h`
`cmd/seed/main.go:66` currently refuses. Build it: customers across the four real
regions (Душанбе, Худжанд, Хорог, Бохтар), suppliers, employees, assets,
documents, a PO through to receipt, an MO through to a released batch, stock
across locations, a trip, inquiries of all five types, deals across all five
stages.

**Done when:** `seed demo` populates every module so no screen is empty;
`APP_ENV=production seed demo` still refuses (`main.go:43`); reference seed
remains untouched and idempotent.

> Schedule this **before** T36 rather than after. The client's first impression of
> the system is the first screen they open, and an empty table reads as a broken
> system to someone who has not been told it is a new one.

#### R15 · Excel / CSV export — `10h`
ToR §4 and §8 condition 7. Nothing exists today; the only export in the system is
the QR ZIP. Build one shared exporter over the existing list endpoints rather than
per-module writers.

**Done when:** every list view has a working export button; the file opens in
Excel with Cyrillic intact (UTF-8 BOM, or XLSX); **the export respects the
caller's permissions and the active filter** — an export that ignores RBAC is a
data leak wearing a download button.

#### R16 · PDF — `8h`
Batch certificate, purchase order, delivery note.

**Done when:** each renders with Cyrillic and Tajik `ҳ ҷ ӯ` correct; a batch
certificate names the batch, its tests, and the releasing user.

#### R17 · Responsive pass — `12h`
Outstanding from T34 (I27). All modules, three breakpoints.

**Done when:** every module is usable at 390px, 768px and 1440px; the website's
assembly line stays horizontal and swipeable on mobile (`CLAUDE.md §5`); no
horizontal body scroll anywhere.

#### R18 · Playwright E2E — `16h`
Outstanding from T34 (I26). The five ToR §5 workflows, driven through the browser
rather than `curl`.

**Done when:** sales, procurement, production and complaint workflows each pass
end to end in CI; each asserts a permission denial as well as a happy path.

#### R19 · Written notice to QOIM — `2h`
Not code, and the only task here with a legal rather than technical consequence.
One document, countersigned:

1. The D7 offline limitation — already owed under D7's own terms.
2. The Финансы и бюджет deferral as a **variation to ToR §8 condition 6**, with a
   dated phase-2 commitment. The decision is settled; the paperwork is not, and
   an internal decision record does not vary a signed acceptance condition.
3. Anything cut under section 6 below.

**Done when:** sent, and acknowledged in writing.

---

## 5. Sequencing

> **Corrected 24 August.** An earlier draft of this file scheduled R00–R19 day by
> day against 9 September. `CLAUDE.md` supersedes that deadline: the sequence is
> **build → internal test → deploy → client test over IP → DNS/TLS → launch**,
> driven by completion rather than dates. The task order below is unchanged —
> it was never really about the calendar — but nothing here is owed on a date.

**185 hours of work**, ordered so that the system is releasable at every point
after Tier 1 rather than only at the end.

| Stage | Tasks | Hours | The system can… |
|---|---|---|---|
| **Scaffold** | R00 · R01 · R02 | 15 | …stop advertising screens it does not have |
| **Tier 1 — the operational chain** | R03 · R04 · R05 · R06 · R07 | 50 | …run a batch from production through release to a loaded lorry |
| **Demo seed** | R14 | 8 | …be shown to the client without empty tables |
| **Tier 2 — registers** | R08 · R09 · R10 · R11 | 34 | …take procurement, documents, maintenance and HR off paper |
| **Tier 3 — CRM** | R12 · R13 | 30 | …hold a customer, a deal and a pipeline that is not permanently empty |
| **Cross-cutting** | R15 · R16 · R17 · R18 | 46 | …export, print, work on a phone, and defend itself against regressions |
| **Notice to QOIM** | R19 | 2 | — |

### Gate: deploy after Tier 1 + R14, not after everything

R14 is scheduled early on purpose. **T36 — the deploy for client testing — should
happen as soon as Tier 1 and the demo seed are done**, not once the whole register
is finished. Three reasons:

1. The client's first impression is the first screen they open. Empty tables read
   as a broken system to someone who has not been told it is a new one.
2. T36/T37/T38 are blocked on things outside this repository. Discovering that the
   server is not ready is much cheaper in week one than in week three.
3. Tier 2 and Tier 3 are the parts most likely to change under client feedback.
   Building them before that feedback arrives is the expensive order.

### Track 0 — start now, it is not code

External lead times that no amount of development speed shortens.

- **Register the domains.** `08-REMAINING-PLAN.md` names this the longest lead
  time in the project. T38 cannot start until the A records resolve.
- **Get the Dushanbe server.** Address, SSH, Docker. T36 is blocked until it exists.
- **Decide where backups are copied to.** `deploy/backup.sh` deliberately does not
  guess, and T35's restore rehearsal cannot close without an answer.
- **Set `PUBLIC_SITE_URL` to the real domain before any wrapper is printed.** QR
  payloads embed it (D11), and codes printed against a bare IP stop resolving.

### The velocity checkpoint

After R08 — roughly half the register by hours — compare actual hours against the
99 estimated for R00–R08. That is the only honest velocity signal this plan
contains, and it arrives early enough to act on.

- **On or ahead** → Tier 3 and R16/R18 are reachable.
- **More than 15% behind** → invoke section 6. Do not absorb the slip by working
  longer; that has already been tried, at 650–750 hours against 280.

---

## 6. Cut lines

Ordered. Cut from the top. Each is chosen so that what it costs is visible to the
developer and invisible, or nearly so, to the factory on 9 September.

1. **R16 PDF** *(−8h)* — nothing in ToR §8 requires PDF specifically; condition 7
   says "exported", which R15 satisfies. Costs a nicety, breaches nothing.
2. **R18 Playwright** *(−16h)* — the largest single saving. Costs regression
   safety after launch, which is a real cost, but the suite protects work that is
   about to stop changing. **Ship the four workflow scripts as a manual test
   checklist instead**, so the coverage exists on paper for T34's gate.
3. **R13 CRM frontend down to a customer register** *(−9h)* — customer list and
   detail, no deal board. `05-MODULES.md:179` already licenses this: *"This module
   will be empty at launch. Build it, seed it lightly, do not invest detail-view
   effort here at the expense of the operational modules."* Keep R12 whole — the
   backend is what makes phase 2 additive.
4. **R11 Персонал to read-only** *(−5h)* — HR data entry can wait a week; the
   register still answers "whose contract expires next", which is what T23 built
   it for.
5. **R10 Оборудование to read-only** *(−4h)* — maintenance can be recorded on
   paper for a fortnight. Say so in R19.
6. **R17 responsive to two breakpoints** *(−5h)* — 390px and 1440px, drop 768px.
   Factory staff are on phones and desk machines; tablets are rare on site.

### Outcome

Everything R00–R21 is done except R19's signature. Nothing was cut.

**Every mutation hook in `lib/operations.ts` is now reachable from the UI.** That
audit — run at the start and at the end — is the single cheapest check in this
plan, and it is the one that would have caught the original defect in twenty
minutes rather than after thirteen tasks were marked done.

Two hooks were REMOVED rather than left unreachable: `useCreateRole` and
`useDeleteRole`. Five roles ship with the system (D9) and are `is_system`;
what an administrator needs is to change a role's permissions and assign roles
to people, both of which exist. Leaving defined-but-unreachable hooks lying
around is precisely the pattern that hid fourteen dead write paths.

**Never cut:** R00, R02, R03, R04, R05, R14, R19. R03 is ToR §8 condition 5 and
the batch cannot legally ship without it. R14 is the client's first impression.
R19 is the only task here whose absence has consequences after the project ends.

---

## 7. Risks

**The estimate is the risk.** 185h is a bottom-up sum of well-understood work with
no allowance for the unknown, and this project has already estimated 650–750 hours
against 280 available (D1). If the D10 checkpoint says 15% behind, that is not
noise — it is the same arithmetic asserting itself again.

**T36–T38 are still blocked on things this repository cannot provide.** The
schedule assumes a server exists by D12 and domains resolve by D16. Neither is in
the developer's control, and D16 has no slack behind it. If the domains slip, the
system launches on a bare IP: acceptable for the CRM, but it must not happen after
QR wrappers are printed.

**No offline mode (D7).** Khorog data entry stops when the connection does, and
the new write paths in R03–R07 are exactly the ones a quality inspector uses.
The accepted risk is unchanged; R19 is what moves it to the client in writing.

**The audit found this in twenty minutes.** It is reasonable to assume the code
holds defects of a similar class that this pass did not reach. R18 is the task
that would find them, and it is second on the cut list — that trade is deliberate,
and it is the least comfortable line in this document.
