#!/usr/bin/env bash
# Generates Go code from proto/**/*.proto into proto/gen/go/.
# Usage: ./scripts/gen-proto.sh   (or: make proto)
#
# Needs protoc 3.21.12, protoc-gen-go 1.36.12, and protoc-gen-go-grpc 1.6.2.
# Pinning these versions keeps generated files reproducible in CI and locally.
set -euo pipefail

cd "$(dirname "$0")/.."

PROTO_DIR=proto
OUT_DIR=proto/gen/go

for tool in protoc protoc-gen-go protoc-gen-go-grpc; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "error: $tool not found on PATH (is \$(go env GOPATH)/bin on PATH?)" >&2
    exit 1
  fi
done

for spec in "protoc|libprotoc 3.21.12" "protoc-gen-go|protoc-gen-go v1.36.12" "protoc-gen-go-grpc|protoc-gen-go-grpc 1.6.2"; do
  tool="${spec%%|*}"
  expected="${spec#*|}"
  actual="$("$tool" --version)"
  actual="${actual/.exe/}"
  if [ "$actual" != "$expected" ]; then
    echo "error: $tool version mismatch: got '$actual', want '$expected'" >&2
    exit 1
  fi
done

mapfile -t files < <(find "$PROTO_DIR" -name '*.proto' -not -path "$OUT_DIR/*" | sort)
if [ "${#files[@]}" -eq 0 ]; then
  echo "No .proto files found."
  exit 0
fi

# Remove previously generated code so deleted .proto files don't leave stale Go behind.
# go.mod and go.sum in OUT_DIR are kept.
find "$OUT_DIR" -name '*.pb.go' -delete
find "$OUT_DIR" -mindepth 1 -type d -empty -delete

protoc -I "$PROTO_DIR" \
  --go_out="$OUT_DIR" --go_opt=paths=source_relative \
  --go-grpc_out="$OUT_DIR" --go-grpc_opt=paths=source_relative \
  "${files[@]}"

echo "Generated Go code for ${#files[@]} proto files into $OUT_DIR"
