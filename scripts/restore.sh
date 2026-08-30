#!/usr/bin/env sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: scripts/restore.sh backups/homeledger-YYYYMMDD-HHMMSS.sql" >&2
    exit 1
fi

docker compose exec -T db psql -U homeledger -d homeledger < "$1"
