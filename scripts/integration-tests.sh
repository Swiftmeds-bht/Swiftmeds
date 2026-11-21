#!/usr/bin/env bash
# Go integration tests (SCRUM-116): every package with `//go:build integration`
# test files, plus the RabbitMQ test in pkg/messaging, run with -race and -v so
# each test shows in the log (for example the SCRUM-37 last-unit concurrency test).
#
# Each test reads its database URL from its own variable and skips without it.
# CI sets them all (.github/workflows/ci.yml) and fails if any test still skips,
# so a missing variable cannot pass unnoticed. Locally, with `make up` running,
# export the same variables (see ci.yml) and run: ./scripts/integration-tests.sh
#
# The saga tests in tests/integration start their own containers and have their
# own CI job, so they are not run here.
set -euo pipefail

cd "$(dirname "$0")/.."

# Directories that hold integration tests.
mapfile -t dirs < <(
  { grep -rlE '^//go:build integration' --include='*_test.go' pkg services
    echo pkg/messaging/consumer_integration_test.go # gated by RABBITMQ_URL, not a build tag
  } | xargs -n1 dirname | sort -u
)

# Group them by Go module, as "./relative/package" paths.
declare -A pkgs=()
for dir in "${dirs[@]}"; do
  module="$dir"
  while [ ! -f "$module/go.mod" ]; do module="$(dirname "$module")"; done
  if [ "$dir" = "$module" ]; then rel="."; else rel="./${dir#"$module"/}"; fi
  pkgs[$module]+=" $rel"
done

log="$(mktemp)"
trap 'rm -f "$log"' EXIT
fail=0
for module in $(printf '%s\n' "${!pkgs[@]}" | sort); do
  echo "::group::Integration tests in ${module}:${pkgs[$module]}"
  # shellcheck disable=SC2086 # the package list is meant to split
  if ! (cd "$module" && GOWORK=off go test -race -tags integration -count=1 -v ${pkgs[$module]}) 2>&1 | tee -a "$log"; then
    fail=1
  fi
  echo "::endgroup::"
done

if [ "${CI:-}" = "true" ] && grep -E -- '--- SKIP' "$log"; then
  echo "::error::Integration tests were skipped (a *_URL variable is missing in ci.yml)."
  fail=1
fi
exit "$fail"
