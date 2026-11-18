#!/usr/bin/env bash
# A development cluster from nothing (overlays/local): k3d cluster, images built
# here and loaded under :dev, secrets from .env, deployment, demo data. Safe to
# run again. Needs Docker, k3d and kubectl (docs/deployment.md, "Install").
#
#   deploy/k8s/overlays/local/up.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT"
CLUSTER=swiftmed-dev
OVERLAY=deploy/k8s/overlays/local

if ! k3d cluster list "$CLUSTER" >/dev/null 2>&1; then
  echo "==> creating the k3d cluster $CLUSTER"
  k3d cluster create --config "$OVERLAY/cluster.yaml"
fi
kubectl config use-context "k3d-$CLUSTER" >/dev/null

echo "==> building the images and loading them into the cluster"
scripts/k8s-images.sh --tag dev --import "$CLUSTER"

[ -f "$OVERLAY/secrets.env" ] || "$OVERLAY/make-secrets.sh"

echo "==> deploying"
redeploy=0
kubectl get namespace swiftmed >/dev/null 2>&1 && redeploy=1
kubectl apply -k "$OVERLAY"
# On a re-deploy, new images under the same :dev tag are not picked up by apply alone.
[ "$redeploy" = 1 ] && kubectl -n swiftmed rollout restart deployment >/dev/null
kubectl -n swiftmed rollout status statefulset/postgres --timeout=5m
kubectl -n swiftmed rollout status statefulset/rabbitmq --timeout=5m
for d in $(kubectl -n swiftmed get deployments -o name); do
  kubectl -n swiftmed rollout status "$d" --timeout=5m
done

echo "==> loading the demo data"
job="seed-$(date +%s)"
kubectl -n swiftmed create job "$job" --from=cronjob/seed
kubectl -n swiftmed wait --for=condition=complete "job/$job" --timeout=5m

kubectl -n swiftmed get pods
cat <<EOF

SwiftMed (development cluster) is running:
  patient app         http://localhost:9100
  pharmacy dashboard  http://localhost:9101
  API gateway         http://localhost:9180/healthz
Demo accounts: patient@demo.swiftmeds.test, demo-tim-001@demo.swiftmeds.test (password swiftmeds-demo).
EOF
