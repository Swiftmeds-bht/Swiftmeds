#!/usr/bin/env bash
# Smoke test for a running `make up` stack (SCRUM-66): search -> reserve -> collect,
# every call through the gateway, the way the web apps make them.
#
#   make smoke            or            ./scripts/smoke-test.sh
#
# Uses the demo data and accounts that `make up` seeds (deploy/seed). On Windows
# without bash, run scripts/smoke-test.ps1 instead.
# Needs only bash and curl. Exit code 0 means every step passed.
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:${GATEWAY_HTTP_PORT:-8080}}"
PASSWORD="${DEMO_PASSWORD:-swiftmeds-demo}"
PATIENT="${DEMO_PATIENT:-patient@demo.swiftmeds.test}"
# Demo Pharmacy Thimphu 02 stocks Demo Ibuprofen 400 in deploy/seed/stock.csv.
PHARMACIST="${DEMO_PHARMACIST:-demo-tim-002@demo.swiftmeds.test}"
PHARMACY_ID="${DEMO_PHARMACY_ID:-40000000-0000-4000-8000-000000000002}"
PRODUCT_ID="${DEMO_PRODUCT_ID:-10000000-0000-4000-8000-000000000003}"
QUERY="${DEMO_QUERY:-Demo Ibuprofen 400}"
LAT=27.4728
LNG=89.6390

step() { printf '\n==> %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
# First value of a JSON string field. Good enough for these flat responses.
field() { sed -n "s/.*\"$1\": *\"\\([^\"]*\\)\".*/\\1/p" | head -n 1; }

login() {
  local body
  body=$(curl -sS -X POST "$GATEWAY/api/v1/auth/login" -H 'content-type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"$PASSWORD\"}") || fail "login request for $1"
  printf '%s' "$body" | field accessToken
}

step "Gateway health"
curl -sSf "$GATEWAY/healthz" >/dev/null || fail "gateway is not reachable at $GATEWAY (is 'make up' running?)"
echo "ok"

step "Search for \"$QUERY\" near Thimphu"
search=$(curl -sS -G "$GATEWAY/api/v1/search" --data-urlencode "q=$QUERY" \
  -d lat=$LAT -d lng=$LNG -d radius=5000)
printf '%s' "$search" | grep -q "\"pharmacyId\": *\"$PHARMACY_ID\"" \
  || fail "search did not return pharmacy $PHARMACY_ID (was the demo data seeded? run 'make seed'). Response: $search"
echo "ok: pharmacy $PHARMACY_ID has it in stock"

step "Sign in as the demo patient and reserve 1 unit"
patient_token=$(login "$PATIENT")
[ -n "$patient_token" ] || fail "patient sign-in failed (run 'make seed' to create the demo accounts)"
hold=$(curl -sS -X POST "$GATEWAY/api/v1/holds" -H "authorization: Bearer $patient_token" \
  -H 'content-type: application/json' \
  -d "{\"pharmacyId\":\"$PHARMACY_ID\",\"productId\":\"$PRODUCT_ID\",\"quantity\":1}")
hold_id=$(printf '%s' "$hold" | field holdId)
[ -n "$hold_id" ] || fail "reserve failed: $hold"
echo "ok: hold $hold_id created"

step "Wait for the reservation saga to confirm the hold"
code=""
for _ in $(seq 1 20); do
  current=$(curl -sS "$GATEWAY/api/v1/holds/$hold_id" -H "authorization: Bearer $patient_token")
  status=$(printf '%s' "$current" | field status)
  case "$status" in
    HOLD_STATUS_ACTIVE) code=$(printf '%s' "$current" | field pickupCode); break ;;
    HOLD_STATUS_REJECTED) fail "hold was rejected (no stock left? run 'make seed' to reset it): $current" ;;
  esac
  sleep 1
done
[ -n "$code" ] || fail "hold did not become ACTIVE within 20 s (last status: ${status:-unknown})"
echo "ok: ACTIVE, pickup code $code"

step "Sign in as the pharmacist of that pharmacy and collect with the pickup code"
pharmacist_token=$(login "$PHARMACIST")
[ -n "$pharmacist_token" ] || fail "pharmacist sign-in failed (run 'make seed' to create the demo accounts)"
collected=$(curl -sS -X POST "$GATEWAY/api/v1/holds/$hold_id/collect" \
  -H "authorization: Bearer $pharmacist_token" -H 'content-type: application/json' \
  -d "{\"pickupCode\":\"$code\"}")
[ "$(printf '%s' "$collected" | field status)" = "HOLD_STATUS_COLLECTED" ] || fail "collect failed: $collected"
echo "ok: COLLECTED"

printf '\nSmoke test passed: search -> reserve -> collect works through the gateway.\n'
