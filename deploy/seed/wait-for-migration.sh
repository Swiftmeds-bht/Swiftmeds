#!/bin/sh
set -eu

database="$1"
table="$2"
attempts=0

while ! psql -U "$POSTGRES_USER" -d "$database" -tAc \
  "SELECT to_regclass('public.$table') IS NOT NULL" 2>/dev/null | grep -q t
do
  attempts=$((attempts + 1))
  if [ "$attempts" -ge 60 ]; then
    echo "Timed out waiting for $database migration table $table" >&2
    exit 1
  fi
  sleep 2
done
