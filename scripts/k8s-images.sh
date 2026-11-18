#!/usr/bin/env bash
# Builds the 23 images the Kubernetes deployment uses (SCRUM-108) with the
# existing Dockerfiles: the 20 services, the two web apps and the seed image.
#
#   scripts/k8s-images.sh                       build, tag :dev
#   scripts/k8s-images.sh --tag v1.0.0 --push   build and push to ghcr.io/swiftmeds-bht
#   scripts/k8s-images.sh --import swiftmed     build, then load into the k3d cluster "swiftmed"
#   scripts/k8s-images.sh --platform linux/arm64 --push   for an ARM host (docker buildx)
#
# Pushing needs `docker login ghcr.io` with a token that has write:packages.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

REGISTRY="${REGISTRY:-ghcr.io/swiftmeds-bht}"
TAG=dev PUSH=0 IMPORT="" PLATFORM=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tag) TAG="$2"; shift 2 ;;
    --push) PUSH=1; shift ;;
    --import) IMPORT="$2"; shift 2 ;;
    --platform) PLATFORM="$2"; shift 2 ;;
    *) echo "unknown option $1" >&2; exit 2 ;;
  esac
done

# name, Dockerfile, build context
images=()
for dockerfile in services/*/Dockerfile; do
  svc="$(basename "$(dirname "$dockerfile")")"
  images+=("$svc|$dockerfile|.")
done
images+=("patient-app|web/patient-app/Dockerfile|web/patient-app")
images+=("pharmacy-dashboard|web/pharmacy-dashboard/Dockerfile|web/pharmacy-dashboard")
images+=("seed|deploy/k8s/base/seed/Dockerfile|deploy")

built=()
for entry in "${images[@]}"; do
  IFS='|' read -r name dockerfile context <<<"$entry"
  ref="$REGISTRY/$name:$TAG"
  echo "==> $ref"
  # The two web apps label their sources for GHCR's "connected repository".
  label=(--label "org.opencontainers.image.source=https://github.com/Swiftmeds-bht/Swiftmeds")
  if [ -n "$PLATFORM" ]; then
    push_flag=(--load); [ "$PUSH" = 1 ] && push_flag=(--push)
    docker buildx build --platform "$PLATFORM" "${push_flag[@]}" "${label[@]}" -f "$dockerfile" -t "$ref" "$context"
  else
    docker build -q "${label[@]}" -f "$dockerfile" -t "$ref" "$context" >/dev/null
    [ "$PUSH" = 1 ] && docker push -q "$ref" >/dev/null
  fi
  built+=("$ref")
done

if [ -n "$IMPORT" ]; then
  echo "==> importing ${#built[@]} images into k3d cluster $IMPORT"
  k3d image import -c "$IMPORT" "${built[@]}"
fi
echo "Done: ${#built[@]} images tagged $TAG$([ "$PUSH" = 1 ] && echo ", pushed to $REGISTRY")."
