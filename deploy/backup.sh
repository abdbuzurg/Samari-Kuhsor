#!/bin/sh
# Nightly database backup.
#
#   ./deploy/backup.sh
#
# Runs pg_dump inside the db container and writes a compressed custom-format dump
# to deploy/backup/. Custom format rather than plain SQL so a single table can be
# restored without replaying everything — the likely restore is "someone deleted
# a batch", not "the disk died".
#
# Install as a cron entry on the server:
#   0 2 * * * cd /opt/samari && ./deploy/backup.sh >> /var/log/samari-backup.log 2>&1
#
# NOTE: this writes to a volume on the SAME server. That is a backup against
# mistakes, not against losing the machine. Copying these files off the box is a
# separate step and is not automated here, because where they go is the client's
# decision and not something to guess at.

set -eu

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi

: "${POSTGRES_USER:?set POSTGRES_USER in .env}"
: "${POSTGRES_DB:=samari}"

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
FILE="samari-${STAMP}.dump"

echo "backup: dumping ${POSTGRES_DB} -> ${FILE}"
docker compose -f docker-compose.prod.yml exec -T db \
  pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Fc -Z 6 \
  > "deploy/backup/${FILE}.partial"

# Renamed only after pg_dump exits 0. A truncated file with a real name is worse
# than no file: it looks like a backup until the day it is needed.
mv "deploy/backup/${FILE}.partial" "deploy/backup/${FILE}"

SIZE=$(wc -c < "deploy/backup/${FILE}")
if [ "${SIZE}" -lt 1024 ]; then
  echo "backup: FAILED — ${FILE} is ${SIZE} bytes, which is not a database" >&2
  exit 1
fi
echo "backup: wrote deploy/backup/${FILE} (${SIZE} bytes)"

# Keep 14 days. Long enough to notice a problem that started last week, short
# enough that the volume does not silently fill and stop Postgres writing.
find deploy/backup -name 'samari-*.dump' -mtime +14 -print -delete
# Partials from an interrupted run are never useful.
find deploy/backup -name '*.partial' -mtime +1 -print -delete
