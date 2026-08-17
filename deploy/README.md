# Deployment

One server in Dushanbe. Docker Compose. Everything below has been rehearsed
locally against the production compose file; nothing here has run on the real
server, because there is not one yet.

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

Set `PUBLIC_SITE_URL` to the server's address, e.g. `http://203.0.113.10`, and
leave `TLS_MODE=off`.

```sh
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml exec api /app/seed
```

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

Migrations are applied by the API on start and ship inside its image, so a
deploy cannot be half-migrated because somebody's laptop lost its connection.

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
