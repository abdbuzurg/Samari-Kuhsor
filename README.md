# Samari Kuhsor platform

CRM/ERP and public website for **QOIM LLC**, brand **Самари Кӯҳсор**, Тем · Хорог · ГБАО ·
Republic of Tajikistan.

| | |
|---|---|
| `backend/` | Go + Postgres 18 API. The only process that opens a database connection. |
| `apps/crm/` | Next.js internal CRM/ERP. Also the CMS for the website. |
| `apps/web/` | Next.js public corporate website. |
| `packages/types/` | TypeScript types generated from Go DTOs by `tygo`. Never hand-edited. |
| `docs/` | Ground truth. Read before writing code. |
| `design/` | **Read-only.** Client-approved prototypes — the visual contract. |

## Getting started

```bash
make up            # Postgres 18 on 127.0.0.1:5433
make db-version    # must report 18.x
make check         # the gate — everything that must be green
make help          # all targets
```

Requires Go 1.26+, Node 24+, Docker with Compose v2+, `goose`, `sqlc`.

## How work proceeds

`TASKS.md` holds the task list, in dependency order. **One task at a time, and `make check` must be
green before the next one opens** — `CLAUDE.md §7` forbids opening a slice with a red suite.

Reading order for a new session:
`CLAUDE.md` → `docs/01-DECISIONS.md` → `docs/07-IMPLEMENTATION-PLAN.md` → the `docs/` file for the
task in hand → `TASKS.md`.

## Things that will trip you up

- **Postgres 18 volume path.** The image requires the mount at `/var/lib/postgresql`, not
  `/var/lib/postgresql/data`. The old path makes the container refuse to start.
- **Archivo has no Cyrillic.** See `docs/07-IMPLEMENTATION-PLAN.md` C1.
- **Tajik is `tg`, never `tj`.** See C2.
- **`Secure` cookies are not sent over plain HTTP.** `TLS_MODE` derives the flag; see I24/I25.
- **Generated code is committed.** `sqlc` and `tygo` output are checked in, and `make check` fails
  if either is stale.
