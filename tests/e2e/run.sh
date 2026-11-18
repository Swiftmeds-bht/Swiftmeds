#!/usr/bin/env bash
# Runs the Cypress end-to-end specs headless against the Docker Compose stack
# (SCRUM-72). Cypress runs in its official Docker image on the stack's network,
# so nothing has to be installed on the machine besides Docker.
#
#   tests/e2e/run.sh --start-stack   build, start and seed the stack with short holds, then test
#   tests/e2e/run.sh                 test a stack that is already running (make up && make seed)
#   tests/e2e/run.sh --spec cypress/e2e/reserve_and_collect.cy.js   extra args go to `cypress run`
#
# The expiry test waits for a hold to run out, so it only runs when holds are
# short (at most 2 minutes). --start-stack starts them with E2E_HOLD_SECONDS
# (default 45) seconds; with a stack started by `make up` (2 hours) it is skipped.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

CYPRESS_IMAGE="${CYPRESS_IMAGE:-cypress/included:16.2.1}"
HOLD_SECONDS="${E2E_HOLD_SECONDS:-45}"
COMPOSE=(docker compose -f deploy/docker-compose.yml --env-file .env)

test -f .env || { echo "Missing .env: run 'cp .env.example .env' first." >&2; exit 1; }

if [ "${1:-}" = "--start-stack" ]; then
  shift
  echo "Starting the stack with ${HOLD_SECONDS}s holds ..."
  # Variables set in the shell win over .env when Compose fills in the file.
  HOLDS_HOLD_DURATION="${HOLD_SECONDS}s" EXPIRY_HOLD_DURATION="${HOLD_SECONDS}s" \
    "${COMPOSE[@]}" up -d --build --wait
  make seed
fi

container() { "${COMPOSE[@]}" ps -q "$1"; }
[ -n "$(container traefik)" ] && [ -n "$(container holds)" ] || {
  echo "The stack is not running: use --start-stack, or 'make up && make seed' first." >&2
  exit 1
}

# How long holds last in the running stack, in seconds (Go duration: 45s, 90m, 2h).
duration="$(docker inspect "$(container holds)" --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | sed -n 's/^HOLDS_HOLD_DURATION=//p')"
case "$duration" in
  *h) hold_seconds=$(( ${duration%h} * 3600 )) ;;
  *m) hold_seconds=$(( ${duration%m} * 60 )) ;;
  *s) hold_seconds="${duration%s}" ;;
  *)  hold_seconds=7200 ;;
esac

network="$(docker inspect "$(container traefik)" \
  --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}' | head -n 1)"
worker_secret="$(sed -n 's/^WORKER_SHARED_SECRET=//p' .env | tail -n 1)"

echo "Running Cypress (${CYPRESS_IMAGE}) on network ${network}, holds last ${hold_seconds}s ..."
# Inside the container, localhost:3000/3001/8080 are forwarded to Traefik
# (scripts/forward.mjs): the apps' Secure cookies only work on localhost.
# Runs as the calling user so screenshots are not owned by root.
docker run --rm \
  --network "$network" \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e E2E_HOLD_SECONDS="$hold_seconds" \
  -e E2E_WORKER_SECRET="$worker_secret" \
  -v "$ROOT/tests/e2e:/e2e" -w /e2e \
  --entrypoint bash "$CYPRESS_IMAGE" -c '
    node scripts/forward.mjs traefik 3000 3001 8080 &
    sleep 1
    exec cypress run --browser electron "$@"
  ' cypress-run "$@"
