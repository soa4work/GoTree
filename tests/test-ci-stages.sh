#!/usr/bin/env bash
# Checks that ci.yml runs every stage of run-tests.sh, and only stages that
# exist, so the hook (all stages) and CI (stages split into jobs) cannot drift.
set -u

SRC="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SRC" || exit 1
fail=0
bad() { echo "FAIL  $1"; fail=1; }

read -r -a stages < <(./run-tests.sh --list-stages)
((${#stages[@]} > 0)) || bad "run-tests.sh --list-stages printed nothing"

# The arguments of every "./run-tests.sh ..." line in ci.yml.
used=" "
while read -r args; do
  for a in $args; do used+="$a "; done
done < <(sed -nE 's#^[[:space:]]*(-[[:space:]]+)?(run:[[:space:]]+)?\./run-tests\.sh([[:space:]]+([^#]*))?$#\4#p' .github/workflows/ci.yml)

[[ "$used" != " " ]] || bad "ci.yml has no ./run-tests.sh stage line"
for s in "${stages[@]}"; do
  [[ "$used" == *" $s "* ]] || bad "ci.yml runs no job for stage '$s'"
done
for a in $used; do
  [[ " ${stages[*]} " == *" $a "* ]] || bad "ci.yml names unknown stage '$a'"
done

./run-tests.sh no-such-stage >/dev/null 2>&1
(($? == 2)) || bad "run-tests.sh did not exit 2 for an unknown stage"

if ((fail)); then echo "❌ ci stage checks failed"; exit 1; fi
echo "✅ ci stage checks passed"
