#!/usr/bin/env bash
# Seals the ngrok authtoken for the fixed API address (SCRUM-156) into
# overlays/prod/sealed-ngrok.yaml, which is safe to commit: only the cluster's
# Sealed Secrets controller can decrypt it. Run it against the deployment
# cluster (like seal-secrets.sh).
#
#   deploy/k8s/overlays/prod/seal-ngrok.sh
#
# The token is read from a hidden prompt and passed on through a pipe: it is
# never echoed, put on a command line or written to disk. Find it at
# dashboard.ngrok.com -> Your Authtoken.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
kubectl get crd sealedsecrets.bitnami.com >/dev/null 2>&1 \
  || { echo "No Sealed Secrets controller in this cluster: start the cluster, or kubectl apply -k deploy/k8s/platform/sealed-secrets" >&2; exit 1; }

read -rsp "ngrok authtoken (hidden): " token; echo
[ -n "$token" ] || { echo "no token given" >&2; exit 1; }

printf '%s' "$token" \
  | kubectl create secret generic swiftmed-ngrok --namespace swiftmed \
      --from-file=NGROK_AUTHTOKEN=/dev/stdin --dry-run=client -o yaml \
  | kubeseal --format yaml --controller-namespace kube-system \
  > "$here/sealed-ngrok.yaml"
unset token
echo "Wrote $here/sealed-ngrok.yaml (encrypted; commit it)."
