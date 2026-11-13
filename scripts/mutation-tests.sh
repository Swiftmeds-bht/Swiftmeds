#!/usr/bin/env bash
# Mutation testing for the core business packages (SCRUM-51, SCRUM-113).
#
# Runs Gremlins on each target package, using the service's .gremlins.yaml
# (every mutator enabled, efficacy and mutant-coverage thresholds), prints one
# row per package and fails if any package is below its thresholds.
#
# Usage: ./scripts/mutation-tests.sh          (or: make mutation)
# One service or package only:
#   MUTATION_TARGETS="holds/internal/app" ./scripts/mutation-tests.sh
#
# Needs Gremlins v0.5.0 on PATH:
#   go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0
set -euo pipefail

cd "$(dirname "$0")/.."

# <service>/<package path inside the service>
MUTATION_TARGETS="${MUTATION_TARGETS:-ranking/internal/domain inventory/internal/domain inventory/internal/app holds/internal/domain holds/internal/app}"

if ! command -v gremlins >/dev/null 2>&1; then
  echo "gremlins not found. Install it with:"
  echo "  go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0"
  exit 2
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0
printf '%-32s %7s %6s %12s %9s %9s  %s\n' PACKAGE KILLED LIVED "NOT COVERED" EFFICACY MCOVER RESULT
for target in $MUTATION_TARGETS; do
  svc="${target%%/*}"
  pkg="./${target#*/}"
  log="$tmp/${target//\//_}.log"

  if [ ! -f "services/$svc/.gremlins.yaml" ]; then
    printf '%-32s %s\n' "$target" "FAIL: services/$svc/.gremlins.yaml is missing"
    fail=1; continue
  fi

  # Gremlins exits non-zero when a threshold in .gremlins.yaml is not met.
  status=0
  (cd "services/$svc" && GOWORK=off gremlins unleash "$pkg") >"$log" 2>&1 || status=$?

  counts="$(grep -E '^Killed: ' "$log" || true)"
  killed="$(sed -nE 's/^Killed: ([0-9]+).*/\1/p' <<<"$counts")"
  lived="$(sed -nE 's/.*Lived: ([0-9]+).*/\1/p' <<<"$counts")"
  notcov="$(sed -nE 's/.*Not covered: ([0-9]+).*/\1/p' <<<"$counts")"
  efficacy="$(sed -nE 's/^Test efficacy: ([0-9.]+%).*/\1/p' "$log")"
  mcover="$(sed -nE 's/^Mutator coverage: ([0-9.]+%).*/\1/p' "$log")"

  if [ -z "$counts" ]; then
    result="FAIL: no result (see output below)"; fail=1
  elif [ "$status" -ne 0 ]; then
    result="FAIL: below threshold"; fail=1
  else
    result=ok
  fi
  printf '%-32s %7s %6s %12s %9s %9s  %s\n' "$target" "${killed:--}" "${lived:--}" "${notcov:--}" \
    "${efficacy:--}" "${mcover:--}" "$result"

  # Show the surviving and uncovered mutants, so they can be killed or explained.
  grep -E '^\s*(LIVED|NOT COVERED|TIMED OUT)' "$log" | sed 's/^ */    /' || true
  if [ -z "$counts" ]; then sed 's/^/    /' "$log"; fi
done

if [ "$fail" -ne 0 ]; then
  echo "Mutation testing FAILED: see the rows above and services/<name>/.gremlins.yaml."
  exit 1
fi
echo "Mutation testing passed."
