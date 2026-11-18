#!/usr/bin/env bash
# Encrypts the deployment's secrets into overlays/prod/sealed-secrets.yaml, which
# is safe to commit: only the cluster's Sealed Secrets controller can decrypt it.
# Run against the deployment cluster (the controller must be installed first:
# kubectl apply -k deploy/k8s/platform/sealed-secrets).
#
#   deploy/k8s/overlays/prod/seal-secrets.sh path/to/secrets.env
#
# The input has the keys of base/secrets.env.example. Write it somewhere outside
# the repository (or use overlays/local/make-secrets.sh, which writes the
# gitignored overlays/local/secrets.env from .env), and delete it afterwards.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
src="${1:?usage: seal-secrets.sh path/to/secrets.env}"
test -f "$src" || { echo "no such file: $src" >&2; exit 1; }

want="$(grep -E '^[A-Z_]+=' "$here/../../base/secrets.env.example" | cut -d= -f1 | sort)"
have="$(grep -E '^[A-Z_]+=' "$src" | cut -d= -f1 | sort)"
missing="$(comm -23 <(echo "$want") <(echo "$have"))"
[ -z "$missing" ] || { echo "missing keys in $src: $missing" >&2; exit 1; }
if grep -q 'change-me' "$src"; then
  echo "refusing: $src still has change-me values; set real secrets for the deployment" >&2
  exit 1
fi

kubectl create secret generic swiftmed-secrets --namespace swiftmed \
    --from-env-file="$src" --dry-run=client -o yaml \
  | kubeseal --format yaml --controller-namespace kube-system \
  > "$here/sealed-secrets.yaml"
echo "Wrote $here/sealed-secrets.yaml (encrypted; commit it)."
