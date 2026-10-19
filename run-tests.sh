#!/usr/bin/env bash
# The one entry point for the whole suite, in four stages:
#
#   shell  the shell tests (no Go, no Node needed)
#   go     gofmt, sqlc, go vet, go test, and govulncheck with GOTREE_AUDIT=1
#   web    npm ci, lint, frontend build (tsc -b: the one type check), vitest
#   e2e    the real binary with the embedded frontend, driven by Playwright
#
# Usage: ./run-tests.sh [stage ...]
#   No argument runs every stage. The branch-guard hook does exactly that
#   before every push. Several stages may be given; they always run in the
#   order above. -h or --help prints this usage.
#
# ci.yml runs the same stages as parallel jobs, with GOTREE_AUDIT=1 and
# GOTREE_RACE=1. tests/test-ci-stages.sh fails when a stage is missing there.
#
# GOTREE_RACE=1 runs the Go tests with the race detector.
#
# GOTREE_AUDIT=1 adds govulncheck and npm audit. They need the network and
# their result changes with every new advisory, so they would make the local
# pre-push gate flaky: CI and the nightly schedule run them, the hook does not.
set -euo pipefail
cd "$(dirname "$0")"

STAGES=(shell go web e2e)

usage() {
  cat <<USAGE
Usage: ./run-tests.sh [stage ...]
       ./run-tests.sh -h | --help

Stages: ${STAGES[*]}
Without a stage every one runs, in that order. Several stages may be given.
USAGE
}

# Hidden: the parity test reads the stage list from here.
if [[ "${1:-}" == --list-stages ]]; then
  echo "${STAGES[*]}"
  exit 0
fi

declare -A selected=()
for arg in "$@"; do
  case "$arg" in
    -h | --help)
      usage
      exit 0
      ;;
    shell | go | web | e2e) selected[$arg]=1 ;;
    *)
      echo "❌ unknown stage '$arg'" >&2
      usage >&2
      exit 2
      ;;
  esac
done
if (($# == 0)); then
  for s in "${STAGES[@]}"; do selected[$s]=1; done
fi
want() { [[ -n "${selected[$1]:-}" ]]; }

need() { # need <tool>: fail early with a pointer to the docs
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ $1 not found — see README 'Development'" >&2
    exit 1
  fi
}

# npm ci and the frontend build are needed by both web and e2e; a run of
# both does each once.
npm_ci_done=0
npm_ci_once() {
  ((npm_ci_done)) && return 0
  echo "📦 npm ci"
  (cd web && npm ci --no-audit --no-fund)
  npm_ci_done=1
}

web_built=0
build_web_once() {
  ((web_built)) && return 0
  npm_ci_once
  # tsc -b is part of the build and is the only type check: it covers src,
  # vite.config.ts and e2e/.
  echo "🏗️  Frontend build (with types)"
  (cd web && npm run build)
  web_built=1
}

stage_shell() {
  echo "🐚 Shell tests"
  ./tests/test-release.sh
  ./tests/test-image-tags.sh
  ./tests/test-pins.sh
  ./tests/test-ci-stages.sh
  ./.claude/hooks/tests/branch-guard-test.sh
}

stage_go() {
  need go

  echo "🎨 gofmt"
  local unformatted
  unformatted="$(git ls-files --cached --others --exclude-standard -- '*.go' | xargs -r gofmt -l)"
  if [[ -n "$unformatted" ]]; then
    echo "❌ Not gofmt-formatted:" >&2
    echo "$unformatted" >&2
    exit 1
  fi

  if [[ -f sqlc.yaml ]]; then
    echo "🗄️  sqlc"
    go tool sqlc diff
  fi

  # web/node_modules can ship Go packages of its own (flatted does), and
  # ./... would vet and test those too.
  local go_pkgs
  mapfile -t go_pkgs < <(go list ./... | grep -v /node_modules/)

  echo "🔍 go vet"
  go vet "${go_pkgs[@]}"

  # The race detector is opt-in (CI sets it): it needs cgo and so a C
  # compiler, and it makes the suite several times slower.
  if [[ "${GOTREE_RACE:-}" == 1 ]]; then
    if ! command -v "$(go env CC)" >/dev/null 2>&1; then
      echo "❌ no C compiler ($(go env CC)) for go test -race — see README 'Development'" >&2
      exit 1
    fi
    echo "🧪 Go tests (race detector)"
    CGO_ENABLED=1 go test -race "${go_pkgs[@]}"
  else
    echo "🧪 Go tests"
    go test "${go_pkgs[@]}"
  fi

  if [[ "${GOTREE_AUDIT:-}" == 1 ]]; then
    echo "🛡️  govulncheck"
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 "${go_pkgs[@]}"
  fi
}

stage_web() {
  need npm
  npm_ci_once

  if [[ "${GOTREE_AUDIT:-}" == 1 ]]; then
    echo "🛡️  npm audit"
    (cd web && npm audit --audit-level=high)
  fi

  echo "🔍 Lint"
  (cd web && npm run lint)

  # Before vitest, so a type error fails early.
  build_web_once

  echo "🧪 Frontend tests"
  (cd web && npm test)
}

stage_e2e() {
  need go
  need npm
  build_web_once

  # The end-to-end tests drive the real binary with the embedded frontend
  # in the system's Chrome at phone and desktop size, with axe on each page.
  echo "🌐 End-to-end tests"
  # Concurrent runs (other worktrees) must not share a port: take a free one
  # unless the caller pinned E2E_PORT.
  if [[ -z "${E2E_PORT:-}" ]]; then
    E2E_PORT="$(node -e 'const s=require("net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')"
  fi
  export E2E_PORT
  CGO_ENABLED=0 go build -o gotree ./cmd/gotree
  (cd web && npm run e2e)
}

ran=()
for s in "${STAGES[@]}"; do
  if want "$s"; then
    "stage_$s"
    ran+=("$s")
  fi
done

if (($# == 0)); then
  echo "✅ All tests passed"
else
  echo "✅ Stages passed: ${ran[*]}"
fi
