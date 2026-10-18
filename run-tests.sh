#!/usr/bin/env bash
# The one entry point for the whole suite: the branch-guard hook runs this
# before every push, and ci.yml has no other step (it only sets GOTREE_AUDIT=1 and
# GOTREE_RACE=1).
#
# GOTREE_RACE=1 runs the Go tests with the race detector.
#
# GOTREE_AUDIT=1 adds govulncheck and npm audit. They need the network and
# their result changes with every new advisory, so they would make the local
# pre-push gate flaky: CI and the nightly schedule run them, the hook does not.
set -euo pipefail
cd "$(dirname "$0")"

echo "🐚 Shell tests"
./tests/test-release.sh
./tests/test-image-tags.sh
./tests/test-pins.sh
./.claude/hooks/tests/branch-guard-test.sh

# The backend and frontend arrive in later milestones; until then each stage
# is skipped rather than failing a checkout that has nothing to test yet.
if [[ -f go.mod ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "❌ go not found — see README 'Development'" >&2
    exit 1
  fi

  echo "🎨 gofmt"
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
    # The store tests alone run close to go test's default 10 minutes under -race.
    CGO_ENABLED=1 go test -race -timeout 30m "${go_pkgs[@]}"
  else
    echo "🧪 Go tests"
    go test "${go_pkgs[@]}"
  fi

  if [[ "${GOTREE_AUDIT:-}" == 1 ]]; then
    echo "🛡️  govulncheck"
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 "${go_pkgs[@]}"
  fi
fi

if [[ -f web/package.json ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "❌ npm not found — see README 'Development'" >&2
    exit 1
  fi

  echo "📦 npm ci"
  (cd web && npm ci --no-audit --no-fund)

  if [[ "${GOTREE_AUDIT:-}" == 1 ]]; then
    echo "🛡️  npm audit"
    (cd web && npm audit --audit-level=high)
  fi

  echo "🔍 Lint & types"
  (cd web && npm run lint && npm run typecheck)

  echo "🧪 Frontend tests"
  (cd web && npm test)

  echo "🏗️  Frontend build"
  (cd web && npm run build)

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
fi

echo "✅ All tests passed"
