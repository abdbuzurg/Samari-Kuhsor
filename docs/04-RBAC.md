# 04 — Permission Model

Resource-based access control. Permissions attach to resources; roles are data composed by
administrators through the UI. The client decides how many roles exist, after launch.

---

## 1. Model

```
user ──many-to-many──▶ role ──many-to-many──▶ (resource, action)
```

A user's effective permission set is the **union** across all their roles. There is no denial
precedence and no role hierarchy — union only. It is simpler to reason about, simpler to test, and
sufficient for a company of this size.

A permission is the pair `resource:action`, e.g. `quality:approve`.

---

## 2. Resources

One resource per module, plus two cross-cutting ones (`admin`, `audit`). Resource keys are stable strings — they
appear in `role_permissions.resource`, in `audit_log.resource`, and in the permission strings
returned by `GET /auth/me`.

| Resource key | Module | Day one |
|---|---|---|
| `dashboard` | Панель управления | ✅ |
| `crm` | CRM и продажи | ✅ |
| `inquiries` | Интеграция с сайтом | ✅ |
| `items` | Товары и цены | ✅ |
| `inventory` | Склад и запасы | ✅ |
| `procurement` | Закупки и поставщики | ✅ |
| `production` | Производство | ✅ |
| `quality` | Качество и безопасность | ✅ |
| `logistics` | Логистика | ✅ |
| `hr` | Персонал | ✅ |
| `equipment` | Оборудование и ТО | ✅ |
| `documents` | Документы | ✅ |
| `finance` | Финансы и бюджет | ❌ deferred, see D2 |
| `cms` | Website content | ✅ |
| `admin` | Users, roles, system settings | ✅ |
| `audit` | Audit log viewer | ✅ |

---

## 3. Actions

| Action | Meaning |
|---|---|
| `read` | View lists, detail pages and reports for this resource |
| `manage` | Create, edit and tombstone records. **Implies `read`.** |
| `approve` | Authorise a state transition that carries authority, not just data entry |

`approve` exists because some transitions are decisions rather than edits. Only these use it:

| Permission | Governs |
|---|---|
| `quality:approve` | Releasing or rejecting a batch out of quarantine, and recalling a released batch |
| `procurement:approve` | Approving a purchase order out of `approval` status |
| `cms:approve` | Moving website content to `approved` / `published` |
| `documents:approve` | Moving a controlled document out of `approval` into `active` |
| `finance:approve` | Approving expense requests *(phase 2)* |

Everywhere else, `read` and `manage` are sufficient.

---

## 4. Seed roles

Ship these so QOIM is not configuring RBAC on opening day. They are `is_system = true` and cannot
be deleted, but their permissions **can** be edited by an administrator — they are a starting point,
not a fixed matrix.

| Resource | Администратор | Директор | Склад | Производство | Качество |
|---|---|---|---|---|---|
| `dashboard` | manage | read | read | read | read |
| `crm` | manage | read | — | — | — |
| `inquiries` | manage | read | — | — | — |
| `items` | manage | read | read | read | read |
| `inventory` | manage | read | manage | read | read |
| `procurement` | manage + approve | read + approve | manage | — | — |
| `production` | manage | read | read | manage | read |
| `quality` | manage + approve | read | read | read | manage + approve |
| `logistics` | manage | read | read | — | — |
| `hr` | manage | read | — | — | — |
| `equipment` | manage | read | read | read | — |
| `documents` | manage + approve | read + approve | read | read | read |
| `cms` | manage + approve | read + approve | — | — | — |
| `admin` | manage | — | — | — | — |
| `audit` | read | read | — | — | — |

Note what Директор does **not** get: `manage` on operational modules. Management reads; the floor
writes. This matches the original synchronisation design, where the site was the system of record.

---

## 5. Enforcement

**Authorization happens in Go middleware. Nowhere else.**

```go
r.Route("/items", func(r chi.Router) {
    r.With(rbac.Require("items", "read")).Get("/", h.ListItems)
    r.With(rbac.Require("items", "manage")).Post("/", h.CreateItem)
    r.With(rbac.Require("items", "manage")).Patch("/{id}", h.UpdateItem)
})

r.With(rbac.Require("quality", "approve")).Post("/batches/{id}/release", h.ReleaseBatch)
```

Rules:

- `manage` satisfies a `read` requirement. `approve` does **not** imply `manage`.
- A missing permission returns **403** with code `forbidden` — never 404, and never a silent empty
  list. Silently filtering results hides bugs.
- The BFF forwards the session and shapes payloads. It makes **no** authorization decisions.
- React uses the permission list from `GET /auth/me` to hide nav entries, buttons and form fields.
  This is presentation only. Never treat a hidden button as a control.
- Every route must declare a permission. A route with no `rbac.Require` is a bug; add a lint or
  startup check that fails if any registered route lacks one.

---

## 6. Role management UI

Not one of the twelve modules, and easy to forget. Required for launch because administrators
compose roles themselves.

- **Roles list** — name, user count, system flag.
- **Role editor** — a permission matrix, resources down, actions across; checkboxes. System roles
  are editable but not deletable.
- **Users list** — name, email, roles, active state, last login.
- **User editor** — assign roles, deactivate, force password reset.
- All of it behind `admin:manage`.

Built alongside it, behind `audit:read`:

- **Audit log viewer** — filterable by actor, resource, resource id and date range, newest first,
  with before/after diffs. The `audit` resource is granted in the seed matrix, so the screen has to
  exist. It is read-only; audit rows are never editable or deletable.

Guardrails:

- The last user holding `admin:manage` cannot be deactivated or stripped of it. Enforce server-side.
- Changing a role's permissions takes effect on the affected users' next request; do not cache
  permissions beyond the request.
- Every role and permission change writes to `audit_log`.

---

## 7. Test requirements

Per `CLAUDE.md`, testing is full coverage. For permissions specifically, every endpoint needs three
integration tests:

1. A user **with** the permission succeeds.
2. A user **without** it gets 403.
3. An unauthenticated request gets 401.

Plus unit tests for permission resolution: union across multiple roles, `manage` implying `read`,
`approve` not implying `manage`, and a user with no roles having no access.

The quarantine transitions in `02-SCHEMA.md` §7 need exhaustive tests — every from/to pair, legal
and illegal, with and without `quality:approve`. This is the regulatory heart of the system.
