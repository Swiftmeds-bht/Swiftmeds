#!/usr/bin/env bash
# Coverage gate for the core services (SCRUM-89).
#
# For each core service, measures statement coverage of its business logic
# (internal/domain and internal/app together) and fails if it is below
# THRESHOLD. Core services that have not been built yet are skipped. Every
# other service is reported but cannot fail the gate.
#
# Usage: ./scripts/coverage-gate.sh          (or: make coverage)
# Override for experiments:
#   CORE_SERVICES="pricing" THRESHOLD=90 ./scripts/coverage-gate.sh
set -euo pipefail

cd "$(dirname "$0")/.."

# Union of the proposal's core services (objective 3) and SCRUM-89's list.
CORE_SERVICES="${CORE_SERVICES:-inventory holds search ranking gateway freshness substitution}"
THRESHOLD="${THRESHOLD:-80}"

is_core() { [[ " $CORE_SERVICES " == *" $1 "* ]]; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Prints "<percent>" or "none" (no domain/app packages) for one service.
measure() {
  local dir="$1" pkgs=()
  for layer in domain app; do
    # Skip placeholder folders (only .gitkeep) so `go test` gets no empty patterns.
    if [ -n "$(cd "$dir" && GOWORK=off go list "./internal/$layer/..." 2>/dev/null)" ]; then
      pkgs+=("./internal/$layer/...")
    fi
  done
  if [ ${#pkgs[@]} -eq 0 ]; then echo none; return; fi

  local coverpkg profile="$tmp/$(basename "$dir").out"
  coverpkg="$(IFS=,; echo "${pkgs[*]}")"
  (cd "$dir" && GOWORK=off go test -count=1 -covermode=atomic \
      -coverpkg="$coverpkg" -coverprofile="$profile" "${pkgs[@]}" >/dev/null)
  (cd "$dir" && GOWORK=off go tool cover -func="$profile") | awk '/^total:/ { sub("%", "", $3); print $3 }'
}

fail=0
printf '%-20s %-6s %9s  %s\n' SERVICE ROLE COVERAGE RESULT
for svc in $CORE_SERVICES; do
  [ -f "services/$svc/go.mod" ] || printf '%-20s %-6s %9s  %s\n' "$svc" core "-" "not built yet, skipped"
done
for mod in services/*/go.mod; do
  dir="$(dirname "$mod")"; svc="$(basename "$dir")"
  role=report; is_core "$svc" && role=core

  if ! cov="$(measure "$dir")"; then
    printf '%-20s %-6s %9s  %s\n' "$svc" "$role" "-" "TESTS FAILED"
    fail=1; continue
  fi
  if [ "$cov" = none ]; then
    if [ "$role" = core ]; then
      printf '%-20s %-6s %9s  %s\n' "$svc" core "-" "FAIL: no internal/domain or internal/app packages"
      fail=1
    else
      printf '%-20s %-6s %9s  %s\n' "$svc" report "-" "no domain/app packages"
    fi
    continue
  fi

  if awk -v c="$cov" -v t="$THRESHOLD" 'BEGIN { exit !(c + 0 >= t + 0) }'; then
    result=ok
  elif [ "$role" = core ]; then
    result="FAIL: below ${THRESHOLD}%"; fail=1
  else
    result="below ${THRESHOLD}% (report only)"
  fi
  printf '%-20s %-6s %8s%%  %s\n' "$svc" "$role" "$cov" "$result"
done

if [ "$fail" -ne 0 ]; then
  echo "Coverage gate FAILED: core services need at least ${THRESHOLD}% on internal/domain + internal/app."
  exit 1
fi
echo "Coverage gate passed (core threshold ${THRESHOLD}%)."
