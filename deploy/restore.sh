#!/bin/sh
# Restore the database from a dump.
#
#   ./deploy/restore.sh deploy/backup/samari-20260817T020000Z.dump
#
# This DESTROYS the current contents of the target database. It refuses to run
# without an explicit confirmation, because the situation in which someone
# reaches for it is also the situation in which they are in a hurry.

set -eu

cd "$(dirname "$0")/.."

DUMP="${1:-}"
if [ -z "${DUMP}" ] || [ ! -f "${DUMP}" ]; then
  echo "usage: ./deploy/restore.sh <dump-file>" >&2
  echo >&2
  echo "available:" >&2
  ls -1t deploy/backup/*.dump 2>/dev/null | head -20 >&2 || echo "  (none)" >&2
  exit 1
fi

if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi
: "${POSTGRES_USER:?set POSTGRES_USER in .env}"
: "${POSTGRES_DB:=samari}"

cat <<WARNING
About to restore ${DUMP}
into database "${POSTGRES_DB}" on this server.

Everything currently in that database will be REPLACED.
WARNING

printf 'Type the database name to confirm: '
read -r CONFIRM
if [ "${CONFIRM}" != "${POSTGRES_DB}" ]; then
  echo "restore: cancelled" >&2
  exit 1
fi

# Stop the writers first. Restoring under a live API produces a database that is
# half old and half new, which is harder to diagnose than either.
echo "restore: stopping api, crm, web"
docker compose -f docker-compose.prod.yml stop api crm web

echo "restore: restoring"
docker compose -f docker-compose.prod.yml exec -T db \
  pg_restore -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" \
  --clean --if-exists --no-owner --exit-on-error \
  < "${DUMP}"

echo "restore: starting services"
docker compose -f docker-compose.prod.yml start api crm web

echo "restore: done. Check the CRM before telling anyone it is finished."
