#!/usr/bin/env bash
# Brings SwiftMed up on the team laptop's k3d cluster (SCRUM-108), from nothing:
# cluster, images, secrets, deployment, demo data. Safe to run again.
#
#   deploy/k8s/local/up.sh
#
# Needs Docker, k3d and kubectl (docs/deployment.md, "Install").
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"
CLUSTER=swiftmed

if ! k3d cluster list "$CLUSTER" >/dev/null 2>&1; then
  echo "==> creating the k3d cluster $CLUSTER"
  k3d cluster create --config deploy/k8s/local/cluster.yaml
fi
kubectl config use-context "k3d-$CLUSTER" >/dev/null

echo "==> building the images and loading them into the cluster"
scripts/k8s-images.sh --tag dev --import "$CLUSTER"

[ -f deploy/k8s/secrets.env ] || deploy/k8s/make-secrets.sh

echo "==> deploying"
redeploy=0
kubectl get namespace swiftmed >/dev/null 2>&1 && redeploy=1
kubectl apply -k deploy/k8s
# On a re-deploy, new images under the same :dev tag are not picked up by apply alone.
[ "$redeploy" = 1 ] && kubectl -n swiftmed rollout restart deployment >/dev/null
kubectl -n swiftmed rollout status statefulset/postgres --timeout=5m
kubectl -n swiftmed rollout status statefulset/rabbitmq --timeout=5m
for d in $(kubectl -n swiftmed get deployments -o name); do
  kubectl -n swiftmed rollout status "$d" --timeout=5m
done

echo "==> loading the demo data"
kubectl -n swiftmed delete job seed --ignore-not-found >/dev/null
kubectl apply -k deploy/k8s/seed-job
kubectl -n swiftmed wait --for=condition=complete job/seed --timeout=5m

kubectl -n swiftmed get pods
cat <<EOF

SwiftMed is running on k3s:
  patient app         http://localhost:9000
  pharmacy dashboard  http://localhost:9001
  API gateway         http://localhost:9080/healthz
Demo accounts: patient@demo.swiftmeds.test, demo-tim-001@demo.swiftmeds.test (password swiftmeds-demo).
EOF
