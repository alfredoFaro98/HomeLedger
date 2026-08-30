#!/usr/bin/env sh
set -eu

backup_dir="${BACKUP_DIR:-./backups}"
timestamp="$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"

docker compose exec -T db pg_dump -U homeledger -d homeledger > "$backup_dir/homeledger-$timestamp.sql"
echo "$backup_dir/homeledger-$timestamp.sql"
