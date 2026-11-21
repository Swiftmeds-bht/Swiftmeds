#!/usr/bin/env bash
# Regenerate protobuf Go code and fail if the checked-in output differs.
set -euo pipefail

cd "$(dirname "$0")/.."
./scripts/gen-proto.sh
if ! git diff --quiet -- proto/gen/go; then
  echo "Generated protobuf code is out of date. Run 'make proto' and commit the result." >&2
  git --no-pager diff --stat -- proto/gen/go >&2
  exit 1
fi
echo "Generated protobuf code matches proto/"
