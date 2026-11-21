#!/usr/bin/env bash
# Coverage gate for every service (SCRUM-89, SCRUM-128).
#
# Measures statement coverage of internal/domain and internal/app together
# for every service and fails if it is below that service's threshold.
#
# Usage: ./scripts/coverage-gate.sh          (or: make coverage)
# Override for experiments:
#   REQUIRED_SERVICES="pricing" THRESHOLD=90 ./scripts/coverage-gate.sh
set -euo pipefail

cd "$(dirname "$0")/.."

# Every deployed Go service has the same minimum threshold by default.
REQUIRED_SERVICES="${REQUIRED_SERVICES:-analytics auth catalog expiry feedback freshness gateway geo holds inventory notification pharmacy-registry pricing ranking search stock-sync substitution trust user-profile watchlist}"
THRESHOLD="${THRESHOLD:-65}"

is_required() { [[ " $REQUIRED_SERVICES " == *" $1 "* ]]; }

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
printf '%-20s %-8s %9s  %s\n' SERVICE ROLE COVERAGE RESULT
for svc in $REQUIRED_SERVICES; do
  [ -f "services/$svc/go.mod" ] || printf '%-20s %-6s %9s  %s\n' "$svc" core "-" "not built yet, skipped"
done
for mod in services/*/go.mod; do
  dir="$(dirname "$mod")"; svc="$(basename "$dir")"
  role=report; is_required "$svc" && role=required

  if ! cov="$(measure "$dir")"; then
    printf '%-20s %-8s %9s  %s\n' "$svc" "$role" "-" "TESTS FAILED"
    fail=1; continue
  fi
  if [ "$cov" = none ]; then
    if [ "$role" = required ]; then
      printf '%-20s %-8s %9s  %s\n' "$svc" required "-" "FAIL: no internal/domain or internal/app packages"
      fail=1
    else
      printf '%-20s %-8s %9s  %s\n' "$svc" report "-" "no domain/app packages"
    fi
    continue
  fi

  if awk -v c="$cov" -v t="$THRESHOLD" 'BEGIN { exit !(c + 0 >= t + 0) }'; then
    result=ok
  elif [ "$role" = required ]; then
    result="FAIL: below ${THRESHOLD}%"; fail=1
  else
    result="below ${THRESHOLD}% (report only)"
  fi
  printf '%-20s %-8s %8s%%  %s\n' "$svc" "$role" "$cov" "$result"
done

if [ "$fail" -ne 0 ]; then
  echo "Coverage gate FAILED: required services need at least ${THRESHOLD}% on internal/domain + internal/app."
  exit 1
fi
echo "Coverage gate passed (all required services threshold ${THRESHOLD}%)."
