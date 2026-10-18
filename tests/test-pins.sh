#!/usr/bin/env bash
# Checks that the build inputs stay pinned (#62): base images by digest, one Go
# version for go.mod, CI, the release and the Docker build, and no checkout
# that leaves its token behind for the steps after it.
set -u

SRC="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SRC" || exit 1
fail=0
bad() { echo "FAIL  $1"; fail=1; }

while read -r line; do
  [[ "$line" =~ @sha256:[0-9a-f]{64}( |$) ]] || bad "Dockerfile: base image not pinned by digest: $line"
done < <(grep -E '^FROM ' Dockerfile)

go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
grep -qE "^FROM .*golang:${go_version//./\\.}-alpine@" Dockerfile \
  || bad "Dockerfile: golang image is not golang:${go_version}-alpine (go.mod says go ${go_version})"

for wf in .github/workflows/*.yml; do
  grep -qE 'go-version:' "$wf" && bad "$wf: set the Go version with go-version-file: go.mod, not go-version"
  checkouts="$(grep -cE 'uses: actions/checkout@' "$wf")"
  kept="$(grep -cE 'persist-credentials: false' "$wf")"
  (( checkouts == kept )) || bad "$wf: $checkouts checkouts but $kept with persist-credentials: false"
done

if (( fail )); then echo "❌ pin checks failed"; exit 1; fi
echo "✅ pin checks passed"
