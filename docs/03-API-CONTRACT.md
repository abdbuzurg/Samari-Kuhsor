# 03 — API Contract

Conventions for the Go backend and the two BFF layers. Follow these exactly; consistency here is
what lets twelve modules be built from one template.

---

## 1. Topology

```
Browser ──httpOnly cookie──▶ Next.js route handler (BFF) ──service token──▶ Go API ──▶ Postgres
```

Rules, restated from `CLAUDE.md` because they are the ones most easily broken:

- The browser **never** calls the Go API. No backend hostname, port or token may appear in any
  client bundle. If a component needs data, it calls `/api/...` on its own Next.js origin.
- The BFF **proxies and shapes**. It does not implement business rules and it does not make
  authorization decisions.
- **Authorization lives in Go middleware.** The BFF forwards the session; Go resolves the user,
  loads permissions, and allows or denies. React may hide a button, but hiding is cosmetic.

---

## 2. Base paths

| Layer | Base |
|---|---|
| Go API | `/api/v1` |
| CRM BFF | `/api` in `apps/crm` |
| Website BFF | `/api` in `apps/web` |

Website BFF routes are deliberately narrow: published content, the product catalogue, and inquiry
submission. It must not proxy arbitrary CRM endpoints.

---

## 3. Resource naming

Plural, kebab-case, module-aligned:

```
/api/v1/items
/api/v1/items/{id}
/api/v1/items/{id}/prices
/api/v1/batches
/api/v1/batches/{id}/quality-tests
/api/v1/batches/{id}/release           POST — requires quality:approve
/api/v1/batches/{id}/reject            POST — requires quality:approve
/api/v1/stock-movements
/api/v1/stock-balances                 read-only, derived
/api/v1/manufacturing-orders
/api/v1/purchase-orders
/api/v1/purchase-orders/{id}/approve   POST — requires procurement:approve
/api/v1/sales-orders
/api/v1/shipments
/api/v1/inquiries
/api/v1/content-pages/{id}/approve     POST — requires cms:approve
/api/v1/content-pages/{id}/publish     POST — requires cms:approve
/api/v1/documents/{id}/approve         POST — requires documents:approve
/api/v1/audit-log                      read-only, requires audit:read
/api/v1/employees
/api/v1/assets
/api/v1/documents
/api/v1/roles
/api/v1/users
/api/v1/content-pages
/api/v1/news-posts
/api/v1/media
```

Standard verbs: `GET` collection, `GET` single, `POST` create, `PATCH` update, `DELETE` tombstone.

**State transitions are sub-resources, not PATCHes.** `POST /batches/{id}/release` rather than
`PATCH /batches/{id} {status:"released"}`. This is what makes permissions and audit entries precise.

Permission strings are always `resource:action` with a colon, everywhere — in code, in
`role_permissions`, and in the list returned by `/auth/me`.

---

## 4. Response envelope

Every successful response:

```json
{
  "data": { },
  "meta": { }
}
```

Collections:

```json
{
  "data": [ ],
  "meta": { "page": 1, "per_page": 50, "total": 212, "total_pages": 5 }
}
```

Every error, at every status code:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Проверьте заполненные поля",
    "details": [
      { "field": "sku", "code": "already_exists", "message": "SKU уже используется" }
    ]
  }
}
```

- `code` is a stable machine string. The frontend switches on `code`, never on `message`.
- `message` is **Russian**, user-facing, safe to display.
- Never leak SQL, stack traces or internal identifiers in `message`.

Codes: `validation_failed` · `unauthenticated` · `forbidden` · `not_found` · `conflict` ·
`version_conflict` · `rate_limited` · `internal_error`.

Status mapping: 200, 201, 400 validation, 401 unauthenticated, 403 forbidden, 404 not found,
409 conflict, 422 business-rule violation, 429, 500.

---

## 5. Collections: pagination, sorting, filtering, search

```
GET /api/v1/items?page=1&per_page=50&sort=-created_at&q=сок&status=active
```

- `page` default 1, `per_page` default 50, maximum 200.
- `sort` is a field name, `-` prefix for descending. Whitelist sortable fields per resource.
- `q` is the live search string the prototype's toolbar sends. Each module declares which columns
  it searches; case-insensitive, unaccented.
- Filters are explicit query parameters per resource. Never accept arbitrary SQL fragments.
- Every collection response is deterministic: always include a tiebreaker sort on `id`.

---

## 6. Timestamps, money, quantities

- All timestamps are **RFC 3339 UTC**: `"2026-09-09T06:30:00Z"`. Formatting for Dushanbe time
  (UTC+5) is the frontend's job.
- **Money is a string**: `"2480000.00"`. Never a JSON number — floats corrupt currency.
- **Quantities are strings** for the same reason: `"8640.000"`.
- Dates without time are `"2026-09-09"`.

---

## 7. Concurrency

Every mutable resource returns `version`. `PATCH` must send the version it read:

```json
PATCH /api/v1/items/{id}
{ "version": 3, "category": "juice" }
```

If the stored version differs, respond `409` with code `version_conflict`. This is cheap now and it
is the mechanism phase-2 synchronisation will rely on.

---

## 8. Status payloads

The prototype renders every status as a coloured tag driven by data. Preserve that shape — the
backend decides the semantic level, not the frontend:

```json
{ "status": { "key": "quarantine", "label": "Карантин", "level": "danger" } }
```

`level` ∈ `ok` | `warn` | `danger` | `info` | `neutral`, per the design contract in `CLAUDE.md`.
Never let a React component map a status string to a colour.

---

## 9. Authentication

| Endpoint | Purpose |
|---|---|
| `POST /api/v1/auth/login` | email + password → session |
| `POST /api/v1/auth/logout` | revoke session |
| `GET /api/v1/auth/me` | current user, roles, resolved permissions |

- Session token is random, stored **hashed**, returned to the BFF and set as an `httpOnly`,
  `Secure`, `SameSite=Lax` cookie by Next.js.
- Passwords are argon2id.
- Lock the account after repeated failures (`failed_attempts`, `locked_until`).
- Idle timeout, absolute expiry, and logout everywhere on password change.
- `GET /auth/me` returns the flat permission list the CRM uses to hide UI:
  `["items:read","items:manage","quality:read","quality:approve", ...]`.

---

## 10. Worked example — `GET /api/v1/items`

```json
{
  "data": [
    {
      "id": "018f3c9e-...",
      "sku": "APJ-1000",
      "item_type": "finished_good",
      "category": "juice",
      "base_uom": "bottle",
      "translations": {
        "ru": { "name": "Яблочный сок прямого отжима" },
        "tg": { "name": "Оби себи табиӣ" },
        "en": { "name": "Cold-pressed apple juice" }
      },
      "packaging_units": [
        { "code": "BOTTLE", "qty_in_base": "1.000", "barcode": null },
        { "code": "CASE12", "qty_in_base": "12.000", "barcode": null }
      ],
      "current_price": { "amount": "18.00", "currency": "TJS" },
      "shelf_life_days": 365,
      "status": { "key": "active", "label": "Активен", "level": "ok" },
      "version": 1,
      "created_at": "2026-08-18T09:14:22Z"
    }
  ],
  "meta": { "page": 1, "per_page": 50, "total": 5, "total_pages": 1 }
}
```

---

## 11. Public website endpoints

Read-only except inquiry submission. Never require a session.

The public surface deliberately says `products` rather than `items` — it is customer-facing
vocabulary. It reads the same `items` table, filtered to `item_type = 'finished_good'`.

```
GET  /api/v1/public/products?locale=ru        only status=active finished goods
GET  /api/v1/public/products/{sku}?locale=ru
GET  /api/v1/public/content/{page_key}?locale=ru   only status=published
GET  /api/v1/public/news?locale=ru
POST /api/v1/public/inquiries
```

`POST /public/inquiries` must:

- validate and rate-limit by IP,
- store the submission,
- return `{ "data": { "reference_no": "WR-0244" } }` — the ToR requires every submission to return
  a reference number,
- appear immediately in the CRM's Интеграция с сайтом module with status `new`.

Prefixes by type: `WR-` wholesale · `CF-` contact · `DA-` distributor · `CP-` complaint ·
`JB-` job application.

---

## 12. Shared types

`packages/types` holds the TypeScript definitions for every payload above, consumed by both Next.js
apps. Generate from the Go types or from an OpenAPI document — do not maintain them by hand in two
places. A drift between Go and TypeScript payload shapes is the most likely silent bug in this
architecture.
