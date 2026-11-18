#!/usr/bin/env bash
# Makes a random Grafana admin password, seals it for this cluster into
# grafana-admin-sealed.yaml (safe to commit), and writes the password itself to
# a file outside the repository that only you can read.
#
#   deploy/k8s/monitoring/make-grafana-secret.sh [~/swiftmed-grafana-admin.txt]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="${1:-$HOME/swiftmed-grafana-admin.txt}"

password="$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 24)"
kubectl create secret generic grafana-admin --namespace monitoring \
    --from-literal=password="$password" --dry-run=client -o yaml \
  | kubeseal --format yaml --controller-namespace kube-system \
  > "$here/grafana-admin-sealed.yaml"
( umask 077; printf 'Grafana admin: user admin, password %s\n' "$password" > "$out" )
echo "Wrote $here/grafana-admin-sealed.yaml (encrypted; commit it) and the password to $out."
