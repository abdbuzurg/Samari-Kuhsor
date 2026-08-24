# Deployment

One server in Dushanbe. Docker Compose.

> **Corrected 24 August, on the server.** An earlier version of this file said
> the stages below had been "rehearsed locally against the production compose
> file". They had not been — the file had been RENDERED (for the topology gate)
> and never booted. Three faults only appeared on the first real `up`:
>
> - `api` was given `DATABASE_URL` and `ADDR`; the binary reads `DB_URL` and
>   `LISTEN_ADDR`, so it exited with "DB_URL is required" and never became
>   healthy, taking `crm`, `web` and `caddy` down with it.
> - `APP_ENV` was hardcoded to `production`, which the I24 boot guard refuses
>   without `TLS_MODE=auto` — making stage 1 impossible to start as written.
> - `api` had no healthcheck at all, so `crm`'s `condition: service_healthy`
>   could never be satisfied.
>
> All three are fixed, and `make check` now runs `tools/check-env-contract.mjs`,
> which fails if the compose file sets a name the binary does not read.

---

## The security model, in one paragraph

`caddy` is the only service with a host port. `api` has none at all — it is
reachable only by the `crm` and `web` containers on the compose network. That is
what makes CLAUDE.md §3's "the browser never calls the Go API directly" a fact
about the network rather than a convention people follow. `SERVICE_KEY` is
defence in depth behind that, never the only lock.

`make check` runs `tools/check-topology.mjs` against the rendered compose config
and fails if any service other than `caddy` publishes a port.

---

## Testing it locally first

Before any of the below, the whole platform runs on a workstation:

```sh
docker compose up --build          # docker-compose.yml, not the .prod one
```

| | |
|---|---|
| Website | http://localhost:3001 |
| CRM | http://localhost:3000 — `admin@samari-kuhsor.tj` / `DevPass!2026` |

That stack is deliberately NOT this topology: every service publishes a port, the
service key is a literal that says `not-for-production`, and the admin password
is written in the compose file. It exists so the thing can be seen working before
it goes anywhere. None of it may be copied to a server — `.env.example` and the
stages below are the real configuration.

---

## Stage 1 — client test over an IP

The domains are not registered yet, so this is where the client sees it first.

```sh
# on the server
git clone <repo> /opt/samari && cd /opt/samari
cp .env.example .env
```

Fill in `.env`:

```sh
openssl rand -base64 32   # POSTGRES_PASSWORD
openssl rand -hex 32      # SERVICE_KEY
```

Set `PUBLIC_SITE_URL` to the server's address, e.g. `http://203.0.113.10`, leave
`TLS_MODE=off`, and set:

```
APP_ENV=staging
```

**`APP_ENV=staging` is required for this stage.** The API refuses to boot with
`APP_ENV=production` unless `TLS_MODE=auto` — the I24 guard at
`cmd/api/main.go:73`, which exists so that going live insecure is impossible
rather than merely discouraged. This stage is deliberately insecure (a bare IP,
no TLS) and is therefore not production. From T38 onward, drop `APP_ENV` from
`.env` entirely: it defaults to `production`, and the guard then bites exactly
when it should.

```sh
docker compose -f docker-compose.prod.yml up -d --build

# Roles, permissions, the five products, the warehouse zones, and the first
# administrator. Idempotent. ADMIN_PASSWORD is read once, here, and never stored.
docker compose -f docker-compose.prod.yml exec \
  -e ADMIN_EMAIL=admin@samari-kuhsor.tj \
  -e ADMIN_PASSWORD='<pick one>' \
  api /app/seed reference
```

The schema is already in place by this point: the API applies its migrations on
start, so `up -d` above has done it.

Then:

| What | Where |
|---|---|
| Public site | `http://<IP>/` |
| CRM | `http://<IP>/crm/` |

### What is deliberately different in this stage

- **Session cookies are not `Secure`.** A `Secure` cookie is never sent over
  plain HTTP, so nobody could log in. `TLS_MODE=off` turns the flag off; the CI
  suite asserts it is ON when `TLS_MODE` is not `off`, so this cannot survive
  into production by accident.
- **`robots.txt` disallows everything.** `app/robots.ts` detects a bare-IP
  `PUBLIC_SITE_URL` and blocks all crawling. A test deployment indexed under an
  address that will stop resolving leaves dead search results for months.
- **Do not print any QR wrappers yet.** The QR payload embeds
  `PUBLIC_SITE_URL`, and wrappers are ordered months in advance (D11). Codes
  printed now would point at an IP that will stop working.

---

## Stage 2 — staging rehearsal with TLS

Optional but recommended before the real switch, because it exercises the
certificate path without depending on DNS being right.

1. Add a hosts-file entry on your own machine pointing a test name at the server.
2. Set `SITE_HOST` and `CRM_HOST` to those names, `TLS_MODE=internal`.
3. Uncomment the two hostname blocks at the bottom of `deploy/Caddyfile` and
   their `tls internal` lines.
4. `docker compose -f docker-compose.prod.yml up -d`

The browser will warn about the certificate — that is correct, it is Caddy's own
CA. What this proves is that the hostname routing, the redirects and the cookie
`Secure` flag all behave, none of which the IP-based stage exercises.

---

## Stage 3 — DNS and public TLS

Only after the domains resolve.

1. Point both A records at the server. Wait for propagation — check from a
   machine that has never resolved the name before, not from one with a cached
   answer.
2. In `.env`: `TLS_MODE=auto`, real `SITE_HOST` / `CRM_HOST`, and an
   `ACME_EMAIL` somebody actually reads. Let's Encrypt warns about expiry to
   that address.
3. In `deploy/Caddyfile`, uncomment the hostname blocks and leave the `tls`
   lines commented — with `TLS_MODE=auto` Caddy provisions certificates itself.
4. Set `PUBLIC_SITE_URL` to `https://<site host>`.
5. `docker compose -f docker-compose.prod.yml up -d`

Then check, in this order:

- [ ] `https://<site>` serves the site with a valid certificate
- [ ] `https://<crm>` serves the CRM, and login works — this is where a
      mis-set `TLS_MODE` shows up, because the cookie is `Secure` now
- [ ] `http://<site>` redirects to HTTPS
- [ ] `curl https://<site>/robots.txt` no longer disallows everything
- [ ] `curl https://<crm>/robots.txt` **does** disallow everything
- [ ] `curl -I http://<IP>:8080` from another machine — must NOT connect

The last one is the important one. If the API answers on a public port, stop and
fix the compose file before anything else.

---

## Backups

```sh
./deploy/backup.sh                      # writes deploy/backup/samari-<stamp>.dump
./deploy/restore.sh deploy/backup/…     # destroys and replaces the database
```

Install the nightly job:

```sh
0 2 * * * cd /opt/samari && ./deploy/backup.sh >> /var/log/samari-backup.log 2>&1
```

**These dumps are on the same server.** They protect against mistakes, not
against losing the machine. Copying them somewhere else is a separate decision
and is deliberately not automated here — where they should go is the client's
call, not something to guess at.

Restore is worth rehearsing once, on the staging stage, before it is ever needed
in anger.

---

## Updating

```sh
cd /opt/samari
git pull
docker compose -f docker-compose.prod.yml up -d --build
```

Migrations are applied by the API on start. They are **embedded in the binary**,
not read from disk, so there is no directory to forget to copy and a deploy
cannot be half-applied because somebody's laptop lost its connection mid-command.

A Postgres advisory lock guards the run, so a second process starting
concurrently waits rather than racing. Failure to migrate is fatal: a server that
starts against a schema it does not understand fails later, further from the
cause, and possibly after writing something.

To inspect a migration before it runs, set `MIGRATE_ON_START=false` and apply it
by hand.

Roll back by checking out the previous tag and rebuilding. Note that a migration
is not automatically reversed — check whether the release added one before
assuming a rollback is clean.

---

## What is NOT here, and why

- **Off-site backup copying.** Needs a destination the client owns.
- **Monitoring and alerting.** Needs somewhere to send an alert.
- **Matomo.** `MATOMO_URL` is empty and the site renders no analytics script
  without it. It can be added later with no code change.
- **A staging server.** There is one machine.

None of these blocks the client test. All of them should be decided before the
factory depends on the system.
