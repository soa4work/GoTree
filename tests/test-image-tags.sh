#!/usr/bin/env bash
# Tests for .github/scripts/image-tags.sh: which moving Docker tags (latest,
# X.Y) a release may take. Needs only bash and coreutils.
set -u

SRC="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$SRC/.github/scripts/image-tags.sh"
fail=0

check() { # tag "existing tags" expected-output
  local got
  got="$(tr " " "\n" <<< "$2" | "$SCRIPT" "$1" | tr "\n" " ")"
  if [[ "$got" == "$3 " ]]; then echo "ok    $1 among [$2]"
  else echo "FAIL  $1 among [$2]: want [$3] got [$got]"; fail=1; fi
}

check v1.0.0  ""                          "latest=true minor=true prerelease=false"
check v1.2.3  "v1.0.0 v1.2.2 v1.2.3"      "latest=true minor=true prerelease=false"
check v1.2.3  "v1.2.3 v1.3.0"             "latest=false minor=true prerelease=false"
check v1.2.2  "v1.2.2 v1.2.3"             "latest=false minor=false prerelease=false"
check v1.10.0 "v1.9.0 v1.10.0"            "latest=true minor=true prerelease=false"
check v1.9.0  "v1.9.0 v1.10.0"            "latest=false minor=true prerelease=false"
check v2.0.0-rc.1 "v1.9.0 v2.0.0-rc.1"    "latest=false minor=false prerelease=true"
check v1.9.0  "v1.9.0 v2.0.0-rc.1"        "latest=true minor=true prerelease=false"
check v0.1.10 "v0.1.9 v0.1.10 v0.10.0"    "latest=false minor=true prerelease=false"

if (( fail )); then echo "❌ image-tags tests failed"; exit 1; fi
echo "✅ image-tags tests passed"
