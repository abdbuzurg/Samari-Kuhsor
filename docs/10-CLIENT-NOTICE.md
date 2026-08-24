# 10 — Written Notice to QOIM

**Status: DRAFT — not yet sent.** R19 is not complete until this is sent and
acknowledged in writing.

Prepared 24 August 2026. Russian translation to be produced before sending; this
English version is the record of what was said.

---

## Why this document exists

Three things about the platform are true, were decided deliberately, and have
consequences the client carries rather than the developer. None of them is a
defect. All three become disputes if the first time QOIM hears about them is
after go-live.

Two of them are owed under the project's own decision record: D7 requires the
offline limitation to be stated in writing "so the risk sits with the client
rather than the contractor". The third is a variation to a signed acceptance
condition and cannot be settled internally at all.

---

## 1. Финансы и бюджет is not in the first release

**What was agreed in the ToR.** Section 2 lists Finance and Budget as a required
functional module. Section 8 lists as a *minimum acceptance condition*:

> Budgets show planned, committed and actual expenditure.

**What is being delivered.** The module is not built. There is no budget table,
no expense request, no approval chain and no budget-versus-actual report.

**Why.** Clarification register question Q2 — whether the system must produce
Tajik statutory, tax-authority-compliant books or only internal management
accounting — has not been answered. The two answers differ by an order of
magnitude in scope. Building against a guess would mean either delivering
something unusable for statutory reporting or absorbing an unbounded amount of
work into a fixed schedule.

**What we are asking QOIM to agree.** That acceptance condition 6 is varied: it
is not part of first-release acceptance, and Финансы и бюджет is delivered in a
later phase on a date agreed once Q2 is answered.

**What this costs QOIM in the meantime.** Budgets, expense approvals and
payables are tracked outside the system. Note that two related figures ARE
available: transport cost per trip is recorded in Логистика, and revenue from
confirmed sales orders appears on the dashboard. Neither is an accounting
record and neither should be used as one.

---

## 2. There is no offline mode (decision D7)

**What this means.** The system runs from a single server in Dushanbe. When
Khorog's internet connection drops, data entry stops — completely. There is no
offline queue, no write buffer, and no read-only cache.

**What is affected.** Every write path: production shift entries, stock
movements, quality test results and batch release decisions.

**Why this matters more than it first appears.** Quality records are the
evidence trail behind the traceability claim. A gap in them during an outage is
not an inconvenience to be filled in later — it is a gap in a regulatory record.

**What was considered and rejected.** A documented paper fallback with catch-up
entry; an IndexedDB write queue for production, stock and QC forms; a read-only
offline cache. Each was rejected to keep the first release deliverable.

**What we recommend QOIM does about it.** Adopt a paper fallback procedure for
QC and production records during outages, and enter them when the connection
returns. This is a process the factory owns; the system does not enforce it and
will not detect a gap.

**Phase 2.** A second on-site instance in Khorog with two-way synchronisation
was designed for and is additive rather than a rebuild — the schema already
carries UUID keys, tombstones and version columns for exactly this. It is not in
the first release.

---

## 3. Items delivered differently from the ToR's wording

Stated for completeness. None of these is expected to be contentious.

| ToR asks for | Delivered | Note |
|---|---|---|
| Excel import/export | **Export** as CSV (UTF-8, opens in Excel) on every register | Import is not built. Nothing in the first release requires bulk loading. |
| Barcode or QR support | **QR** for batches, with a printer handoff export | Product barcodes (EAN/GTIN) are not implemented — the products have no assigned GTINs yet. |
| Optional two-factor authentication | Not implemented | Explicitly optional in the ToR. Sessions expire after 8 hours idle. |
| Recipes / BOM in Производство | Not implemented | Compositions are `уточняется` until recipes are approved and lab-verified — a client instruction. The system must not publish unverified claims. Production orders, batches, consumption and yield are all delivered. |
| HR: attendance, leave, training, payroll export | Employee register with contracts and shifts only | The wider HR module is phase 2. |
| Procurement: purchase requests, quotation comparison, supplier performance | Purchase orders, approval ladder and goods receipt | The tendering side is phase 2. |
| Quality: sanitation logs, nonconformity/CAPA register | Incoming and in-process test results, quarantine and release | The release gate — acceptance condition 5 — is delivered in full. |

---

## 4. What IS delivered against ToR §8

| # | Acceptance condition | Status |
|---|---|---|
| 1 | Website inquiries create CRM leads | ✅ |
| 2 | Orders trackable from inquiry through delivery | ✅ (payment excluded, see §1) |
| 3 | Finished products traceable to raw-material batches | ✅ |
| 4 | Warehouse balances by item, batch and expiry | ✅ |
| 5 | Quality staff can quarantine and release finished goods | ✅ |
| 6 | Budgets show planned, committed and actual | ❌ — variation requested, §1 |
| 7 | Reports exportable · permissions enforced · backups restorable | ✅ export · ✅ permissions · ⚠️ restore not yet rehearsed on the production server |

Item 7's restore rehearsal is blocked on the Dushanbe server existing. The
scripts and the procedure are written and will be rehearsed as the first act
after deployment, before any real data is entered.

---

## 5. What we need back

1. Countersignature on the variation in §1.
2. Acknowledgement of §2, and confirmation of who owns the paper fallback procedure.
3. Acknowledgement of §3.

---

## Appendix — what QOIM needs to provide

Not contractual, but each blocks a step and none is in the developer's control.

- **The Dushanbe server.** Address, SSH access, Docker installed.
- **The two domain registrations,** with A records pointed at that server. This
  is the longest external lead time in the project.
- **A backup destination** — where nightly database and upload archives are
  copied to. The backup script deliberately does not guess.
- **Confirmation of the public site URL before any wrappers are printed.** QR
  payloads embed it and wrappers are ordered months in advance; codes printed
  against a temporary address will stop resolving.
