#!/bin/sh
# Loads the demo fixtures (SCRUM-66). Runs in the one-shot "seed" container that
# `make up` and `make seed` start; see deploy/docker-compose.yml.
#
# Compose starts this only after Auth, Catalog, Pharmacy Registry, Geo, Inventory
# and Pricing report healthy, and each of them applies its migrations before it
# does, so the tables already exist. The wait below is a second guard for when
# the script is run some other way.
#
# Connection: PGHOST, PGUSER and PGPASSWORD come from the container environment.
set -eu

dir=/opt/swiftmed/seed

wait_for_table() {
  database="$1"
  table="$2"
  attempts=0
  until psql -d "$database" -tAc "SELECT to_regclass('public.$table') IS NOT NULL" 2>/dev/null | grep -q t; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 60 ]; then
      echo "seed: timed out waiting for table $table in $database" >&2
      exit 1
    fi
    sleep 2
  done
}

load() {
  database="$1"
  table="$2"
  file="$3"
  wait_for_table "$database" "$table"
  echo "seed: $file -> $database"
  psql -q -v ON_ERROR_STOP=1 -d "$database" -f "$dir/$file"
}

# Catalog medicines are loaded by services/catalog/migrations/002_seed_medicines.up.sql.
wait_for_table catalog_db catalog_products

load pharmacy_registry_db pharmacies          pharmacy-registry.sql
load geo_db               pharmacy_locations  geo.sql
load inventory_db         inventory_stock     inventory.sql
load pricing_db           prices              pricing.sql
load auth_db              users               auth.sql

echo "seed: done. Demo data is synthetic and for development and testing only."
echo "seed: sign in as patient@demo.swiftmeds.test or demo-tim-001@demo.swiftmeds.test (password: swiftmeds-demo)."
