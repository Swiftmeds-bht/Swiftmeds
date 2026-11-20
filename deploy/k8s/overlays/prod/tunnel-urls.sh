#!/usr/bin/env bash
# Prints the public URLs of the quick tunnels (SCRUM-75), and with
# --deploy-workers points both Cloudflare Workers at the current API URL.
#
#   deploy/k8s/overlays/prod/tunnel-urls.sh
#   CLOUDFLARE_API_TOKEN=... deploy/k8s/overlays/prod/tunnel-urls.sh --deploy-workers
#
# A quick tunnel's URL changes when the public-tunnel pod restarts (for example
# after the laptop restarts), so run it with --deploy-workers again then.
#
# Deploying needs a Cloudflare API token with the "Edit Cloudflare Workers"
# template (dash.cloudflare.com -> My Profile -> API Tokens), in
# CLOUDFLARE_API_TOKEN. The workers' WORKER_SHARED_SECRET is set from the
# repository's .env on the first deploy, or always with --set-secret. It must
# equal the gateway's (the cluster's secrets come from the same .env).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
NS=swiftmed

url_of() {
  kubectl -n "$NS" logs deploy/public-tunnel -c "$1" 2>/dev/null \
    | grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' | tail -n 1
}

patient="$(url_of patient)"; api="$(url_of api)"
if [ -z "$patient" ] || [ -z "$api" ]; then
  echo "The tunnels have no URL yet: kubectl -n $NS get pods -l app.kubernetes.io/name=public-tunnel" >&2
  exit 1
fi
cat <<EOF
SwiftMed            $patient
Pharmacy portal     $patient/pharmacy-portal
API gateway         $api
EOF

deploy=0 set_secret=0
for arg in "$@"; do
  case "$arg" in
    --deploy-workers) deploy=1 ;;
    --set-secret) deploy=1; set_secret=1 ;;
    *) echo "unknown option $arg" >&2; exit 2 ;;
  esac
done
[ "$deploy" = 1 ] || exit 0

: "${CLOUDFLARE_API_TOKEN:?set CLOUDFLARE_API_TOKEN (see the comment at the top)}"
secret="$(sed -n 's/^WORKER_SHARED_SECRET=//p' "$ROOT/.env" | tail -n 1)"
[ "${#secret}" -ge 16 ] || { echo "WORKER_SHARED_SECRET in $ROOT/.env must be at least 16 characters" >&2; exit 1; }

for worker in hold-expiry freshness-decay; do
  echo "==> $worker -> $api"
  ( cd "$ROOT/workers/$worker"
    [ -d node_modules ] || npm ci --no-audit --no-fund --loglevel=error
    npx wrangler deploy --var "GATEWAY_URL:$api"
    if [ "$set_secret" = 1 ] || ! npx wrangler secret list 2>/dev/null | grep -q WORKER_SHARED_SECRET; then
      printf '%s' "$secret" | npx wrangler secret put WORKER_SHARED_SECRET
    fi )
done
echo "Both workers call $api. In a minute, check (expect status 200):"
echo "  kubectl -n $NS logs -l app.kubernetes.io/name=traefik --since=5m --tail=-1 | grep -c 'internal/expiry/sweep'"
