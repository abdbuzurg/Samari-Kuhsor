# 01 — Decision Record

Decisions taken for the Samari Kuhsor platform. **These are settled.** Do not reopen them, do not
propose alternatives, and do not silently implement something different.

Where a decision carries known risk, the risk is stated. Stating it is not an invitation to
relitigate the decision — it is there so nobody is surprised later.

Last updated: 17 August 2026.

---

## D1 — Both systems go live on 9 September 2026

The website and the CRM/ERP are both in production on opening day. The factory opens the same day.

**Rejected:** phasing the CRM to late September/October, shipping the CRM as a demo instance.

**Accepted risk:** this is a solo build. The work was estimated at 650–750 hours against roughly
280 available. The developer has accepted this knowingly.

---

## D2 — Twelve modules hold real operational data; Финансы и бюджет does not

*(Twelve operational modules, plus the CMS described in D5 — thirteen surfaces in total. Finance is
the only one of the thirteen deliberately excluded.)*

Live with real data on day one: Панель управления, CRM и продажи, Интеграция с сайтом,
Товары и цены, Склад и запасы, Закупки и поставщики, Производство, Качество и безопасность,
Логистика, Персонал, Оборудование и ТО, Документы.

**Deferred:** Финансы и бюджет, pending the client's answer to register question Q2 — whether the
ERP must produce Tajik statutory, tax-authority-compliant books, or only internal management
accounting. Until that is answered the module's size is unbounded, so it is quarantined.

The schema may include finance table stubs so nothing has to be restructured later, but no finance
UI or business logic ships for 9 September.

---

## D3 — Stack: Go + Postgres + sqlc behind two Next.js apps

- One Go backend, Postgres, sqlc for query codegen, goose for migrations.
- `apps/crm` — Next.js. Internal CRM/ERP, and the CMS for the website.
- `apps/web` — Next.js. Public website.
- Both frontends reach the backend **only** through their own Next.js route handlers acting as a
  BFF, so no credential or backend URL is ever exposed to the browser.

**Rejected:** batteries-included frameworks (Django/DRF, Laravel, NestJS) that would have supplied
auth, RBAC, migrations and an admin panel for free; collapsing the backend into Next.js route
handlers.

**Consequence:** authentication, sessions, the permission system, migrations tooling and all CRUD
UI are hand-built. There is no admin panel, so every module needs a bespoke interface.

---

## D4 — Monorepo

`backend/`, `apps/crm`, `apps/web`, `packages/types`, `docs/`, `design/` in one repository.
Chosen because the CRM drives website content, so changes routinely span all three applications.

---

## D5 — Both prototypes are ported to Next.js before launch

The approved vanilla-DOM prototypes are rebuilt as React. The website's imperative animations —
staggered belt roll-in, the three-stage SVG map border draw, the marquee, replay-on-view-return,
reduced-motion fallbacks — are rebuilt around refs and effects.

**Rejected:** shipping the website as the existing standalone build with only a form endpoint added,
and porting it after launch.

**Consequence:** the CMS ships on day one, so the CRM drives website content from launch. Cost is
roughly 70–110 hours of port work that produces no new functionality.

---

## D6 — Single Dushanbe instance at launch. No Khorog install, no sync.

One deployment in Dushanbe serving both Khorog staff and remote management. The on-site Khorog
instance and the Khorog↔Dushanbe synchronisation are **phase 2**.

**Rejected:** full two-way hourly sync at launch; one-way push with a read-only cloud mirror;
a narrow cloud→site command queue.

**Consequence:** Khorog staff depend on their internet connection for all data entry.

---

## D7 — The offline gap is accepted, not mitigated

When Khorog's connection drops, data entry stops. There is no offline queue, no PWA write buffer,
no read-only cache, and no paper fallback procedure.

**Rejected:** documented paper fallback with catch-up entry; IndexedDB write queue for production,
stock and QC forms; read-only offline cache.

**Accepted risk:** an outage interrupts batch records, stock movements and QC results. Quality
records are the evidence trail behind the traceability claim, so a gap in them is a compliance gap.

**Action required of the developer:** state this limitation to QOIM in writing before launch, so
the risk sits with the client rather than the contractor.

---

## D8 — The catalogue is exactly five products

| SKU | Name | Volume | Packaging |
|---|---|---|---|
| `APJ-1000` | Яблочный сок прямого отжима | 1 000 мл | Стеклянная бутылка |
| `APR-220` | Абрикосовый джем | 212–228 мл | Стеклянная банка |
| `TOM-500` | Томатная паста | 500 мл | Стеклянная банка |
| `WAT-500` | Негазированная питьевая вода 0,5 л | 500 мл | ПЭТ-бутылка |
| `WAT-1000` | Негазированная питьевая вода 1 л | 1 000 мл | ПЭТ-бутылка |

These are the products the client approved on the website prototype. The CRM prototype's
pomegranate juice, apricot juice, strawberry jam and 1.5 л water were **design filler and are not
real**. They must not appear anywhere in the system.

Rules that follow:

- **Finished-goods SKUs keep the approved codes above.** Do not renumber them to the CRM
  prototype's `JUS-APL-100` grammar — the client has already seen and approved these.
- **Non-finished goods get their own prefixes**: `RAW-` raw materials (e.g. `RAW-SUG-50`),
  `PKG-` packaging materials (e.g. `PKG-CAP-82`).
- **A case is not a product.** One product record has a base consumer unit plus defined packaging
  units. `WAT-500 × 24` is a selling unit of `WAT-500`, not a separate SKU. The CRM displays cases
  where it counts pallets; the website displays the bottle.

---

## D9 — Resource-based RBAC, roles composed by the client

Permissions attach to **resources** (modules), with actions `read`, `manage` and `approve`.
Administrators create roles and assign permissions through the UI; the number of roles is therefore
decided by the client after launch, not fixed in code.

Requirements that follow:

- A **role management screen** must be built — role editor, permission matrix, user-to-role
  assignment. It is not one of the twelve modules and is easy to forget.
- **Seed roles ship with the system** so QOIM is not configuring RBAC on opening day:
  Администратор, Директор, Склад, Производство, Качество.
- `approve` exists because releasing a batch from quarantine is a different authority from editing
  a QC record. See `04-RBAC.md`.

---

## D10 — All three languages are live on 9 September

Russian, Tajik and English, on both the website and the CRM interface.

**Consequence:** translation is the longest external dependency in the project. The full Russian
website copy plus CRM chrome must go to translators, come back, and pass linguist review inside the
remaining calendar. This work is not under the developer's control and must start immediately.

---

## D11 — QR code generation must exist before 9 September

Register question Q4 covers two separate things: the internal QR code, settled here, and retail
EAN-13 barcodes, still open — see the table at the end of this document.

On the QR: the client does not print its own wrappers. Wrappers are ordered externally against a
planned batch volume, in advance. The QR code is generated in the CRM and handed to the printing
company.

**Consequence:** QR generation is needed *before* the plant produces anything, not from launch
onward. The wrapper lead time may already have started; confirm with QOIM immediately.

---

## Open questions still owed by the client

From `QOIM_Samari_Kuhsor_Clarification_Register.docx` (v2):

| Ref | Question | Impact if unanswered |
|---|---|---|
| **Q2** | Accounting depth — statutory Tajik books or internal management accounting? | Blocks Финансы и бюджет entirely. Highest priority. |
| Q3 | VAT handling, invoice formats, multi-currency | Blocks invoicing design |
| Q4 *(second part)* | Retail EAN-13 barcodes, separate from the internal QR settled in D11 | Shelf-ready packaging may be non-compliant. `packaging_units.barcode` stays null until answered. |
| Q6 | Warranty period and paid maintenance SLA | Contractual, not technical |
| Q9–Q12, Q14, Q15 | Awaiting client response | Various |

## Content still owed by the client

Real product photography · retailer logos · the company phone number (`+992 —` is incomplete) ·
confirmation of `info@samari-kuhsor.tj` · Pamir landscape photography · news article content and
images · technical datasheet PDFs · legal page copy for the privacy policy, terms and cookie pages ·
Tajik and English translations.

Every one of these is on the launch critical path.
