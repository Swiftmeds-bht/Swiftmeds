#!/usr/bin/env bash
# Opens the Headlamp dashboard (SCRUM-161) at http://localhost:4466 and prints a
# read-only login token. Install it first:
#   kubectl apply -k deploy/k8s/platform/headlamp
# Usage: deploy/k8s/platform/headlamp/open.sh [port]     (Ctrl+C to stop)
set -euo pipefail

PORT="${1:-4466}"
HOURS="${HEADLAMP_TOKEN_HOURS:-8}"

if ! kubectl get namespace headlamp >/dev/null 2>&1; then
  echo "Headlamp is not installed: kubectl apply -k deploy/k8s/platform/headlamp" >&2
  exit 1
fi
kubectl -n headlamp rollout status deploy/headlamp --timeout=120s >/dev/null

cat <<EOF
Headlamp: http://localhost:${PORT}
Log in with this token (read-only, valid ${HOURS} h). Pick the "swiftmed"
namespace at the top to see the app.

EOF
kubectl -n headlamp create token headlamp-viewer --duration="${HOURS}h"
echo
echo "Ctrl+C stops the dashboard."
exec kubectl -n headlamp port-forward --address 127.0.0.1 svc/headlamp "${PORT}:80"
