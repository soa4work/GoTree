#!/usr/bin/env bash
# Decides which moving Docker tags a release tag may take. Reads every v* tag
# of the repository on stdin and prints GitHub step outputs:
#
#   latest=true      TAG is the newest stable release, so it may move :latest
#   minor=true       TAG is the newest stable release of its X.Y line, so it
#                    may move :X.Y
#   prerelease=true  TAG is not a plain vX.Y.Z (v1.2.0-rc.1, ...)
#
# Re-running the release of an older tag, or releasing a prerelease, must not
# move :latest or :X.Y backwards (#62).
#
# Usage: git tag -l 'v*' | .github/scripts/image-tags.sh v1.2.3 >> "$GITHUB_OUTPUT"
set -euo pipefail

tag="${1:?usage: image-tags.sh <tag> < tags}"
stable='^v[0-9]+\.[0-9]+\.[0-9]+$'

mapfile -t tags < <({ cat; printf '%s\n' "$tag"; } | grep -E "$stable" | sort -uV)

latest=false minor=false prerelease=true
if [[ "$tag" =~ $stable ]]; then
  prerelease=false
  [[ "$tag" == "${tags[-1]}" ]] && latest=true
  line="${tag%.*}."
  newest_in_line="$(printf "%s\n" "${tags[@]}" | grep -E "^${line//./\\.}[0-9]+$" | tail -n 1)"
  [[ "$tag" == "$newest_in_line" ]] && minor=true
fi

echo "latest=$latest"
echo "minor=$minor"
echo "prerelease=$prerelease"
