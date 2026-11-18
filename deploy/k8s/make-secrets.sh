#!/usr/bin/env bash
# Writes deploy/k8s/secrets.env (gitignored) from the repository's .env, keeping
# only the secret keys listed in secrets.env.example. The in-cluster host names
# (postgres, rabbitmq) are the same as in Docker Compose, so the values carry over.
#
#   deploy/k8s/make-secrets.sh [path/to/.env]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
src="${1:-$here/../../.env}"
test -f "$src" || { echo "No $src: run 'cp .env.example .env' and change the secrets first." >&2; exit 1; }

keys="$(grep -E '^[A-Z_]+=' "$here/secrets.env.example" | cut -d= -f1)"
out="$here/secrets.env"
umask 077
: > "$out"
missing=""
for key in $keys; do
  line="$(grep -E "^${key}=" "$src" | tail -n 1 || true)"
  if [ -z "$line" ]; then missing="$missing $key"; continue; fi
  echo "$line" >> "$out"
done
if [ -n "$missing" ]; then
  echo "Missing in $src:$missing" >&2
  rm -f "$out"
  exit 1
fi
if grep -q 'change-me' "$out"; then
  echo "warning: $out still has change-me values; fine on a laptop, not for a shared demo." >&2
fi
echo "Wrote $out ($(wc -l < "$out") keys)."
