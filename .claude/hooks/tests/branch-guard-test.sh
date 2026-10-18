#!/usr/bin/env bash
# Tests for .claude/hooks/branch-guard.sh: the workflow rules it enforces
# (issue branches, no pushes to main, tests before a push) and the cases that
# have bypassed it before — quoted text, heredocs, command wrappers, worktrees.
#
# Runs against a throwaway clone with a stubbed run-tests.sh; nothing is built,
# pushed or tested for real.
set -u

SRC="$(cd "$(dirname "$0")/../../.." && pwd)"

# The fixtures commit, and a CI runner has no git identity of its own.
export GIT_AUTHOR_NAME=Tester GIT_AUTHOR_EMAIL=tester@example.com
export GIT_COMMITTER_NAME=Tester GIT_COMMITTER_EMAIL=tester@example.com

# A CI checkout is a detached HEAD, so a clone of it has no branch at all and no
# origin/main to track. Both fixtures need main and its upstream to exist, or
# every case in this file is judged against a branch that is not there.
fixture_main() {
  git checkout -q -B main
  git update-ref refs/remotes/origin/main HEAD
  git branch -q --set-upstream-to=origin/main main
}

W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
git clone -q "$SRC" "$W/r" && { cd "$W/r" || exit 1; }
fixture_main && cp -r "$SRC/.claude" . && cp "$SRC/run-tests.sh" . && git add -A && git commit -qm fixture
export CLAUDE_PROJECT_DIR="$W/r"
G="git"; C="commit"
fail=0

check() { # expected tool arg label
  local exp="$1" tool="$2" arg="$3"
  local got
  got=$(jq -nc --arg t "$tool" --arg a "$arg" --arg cwd "$W/r" \
    'if $t=="Bash" then {tool_name:$t,cwd:$cwd,tool_input:{command:$a}} else {tool_name:$t,cwd:$cwd,tool_input:{file_path:$a}} end' \
    | .claude/hooks/branch-guard.sh | jq -r '.hookSpecificOutput.permissionDecision // empty')
  got=${got:-allow}
  if [[ "$got" == "$exp" ]]; then echo "ok    $exp  ${arg//$'\n'/⏎}"; else echo "FAIL  want $exp got $got  ${arg//$'\n'/⏎}"; fail=1; fi
}

echo "== on main"
check deny  Write "$W/r/README.md"
check allow Write "/tmp/x"
check allow Bash  "$G status"
check deny  Bash  "$G $C -m x"
check deny  Bash  "$G switch -c foo"
check deny  Bash  "$G switch -qc foo"
check deny  Bash  "$G checkout -qb foo"
check allow Bash  "$G merge --ff-only origin/main"
check allow Bash  "$G reset --hard origin/main"
check deny  Bash  "$G merge origin/main"
check deny  Bash  "$G merge --ff-only some-other-branch"
check deny  Bash  "$G reset --hard HEAD~1"
check allow Bash  "$G merge --ff-only origin/main 2>&1 | tail -1"
check allow Bash  "$G merge --ff-only origin/main > /tmp/out.txt"
check deny  Bash  "$G merge origin/main 2>&1"
check allow Bash  "$G switch -qc fix/42-audio && $G $C -m x"
check allow Bash  "$G switch -c fix/42-audio && $G add -A && $G $C -m x"
check deny  Bash  "$G push origin main"
check deny  Bash  "$G push origin fix/42-audio:main"
check ask   Bash  "gh pr merge 3"
check ask   Bash  "GH_PAGER=cat gh pr merge 3"
check allow Bash  "echo $G $C"
check allow Bash  "bash -c \"echo $G add -A\""
check allow Bash  "cat > $W/f <<'EOF'
text
$G add -A
EOF
echo done"
check deny  Bash  "cat > $W/f <<'EOF'
text
EOF
$G add -A"
check deny  Bash  "echo a
$G $C -m x"
# A "<<" that opens no heredoc must not hide the lines after it.
check deny  Bash  "echo \"<<X\"
$G $C -m x"
check deny  Bash  "echo '<<X'
$G $C -m x"
check deny  Bash  "echo \$((1<<2))
$G $C -m x"
check deny  Bash  "cat <<<word
$G $C -m x"
check allow Bash  "$G -C /tmp/other $C -m x"
check deny  Bash  "$G -C . $C -m x"
check deny  Bash  "time $G $C -m x"
check deny  Bash  "env FOO=1 $G push origin main"
check deny  Bash  "sudo $G $C -m x"
check ask   Bash  "time gh pr merge 3"

echo "== on main: bypasses that used to slip through"
# Code inside quotes that a shell runs is code.
check deny  Bash  "bash -c \"$G push origin main\""
check deny  Bash  "sh -c '$G $C -m x'"
check deny  Bash  "bash -lc \"echo x && $G add -A\""
check deny  Bash  "eval \"$G $C -m x\""
check deny  Bash  "echo \$($G $C -m x)"
check deny  Bash  "echo \"\$($G $C -m x)\""
check deny  Bash  "echo \`$G $C -m x\`"
check deny  Bash  "bash <<'EOF'
$G $C -m x
EOF"
check deny  Bash  "env -S '$G $C -m x'"
check deny  Bash  "( $G $C -m x )"
check deny  Bash  "{ $G $C -m x; }"
check deny  Bash  "if $G $C -m x; then echo; fi"
check deny  Bash  "timeout 60 $G $C -m x"
check deny  Bash  "nice -n 5 $G $C -m x"
check deny  Bash  "sudo -u root $G $C -m x"
# Another spelling of git is still git.
check deny  Bash  "/usr/bin/$G $C -m x"
check deny  Bash  "\\$G $C -m x"
check deny  Bash  "echo x | xargs $G $C -m"
check deny  Bash  "echo main | xargs $G push origin"
check deny  Bash  "$G -c alias.p=push p origin main"
check ask   Bash  "$G --git-dir=$W/r/.git $C -m x"
check ask   Bash  "GIT_DIR=$W/r/.git $G $C -m x"
# Every refspec that lands on main.
check deny  Bash  "$G push origin +main"
check deny  Bash  "$G push origin :main"
check deny  Bash  "$G push origin HEAD:refs/heads/main"
check deny  Bash  "$G push --force origin +fix/42-audio:main"
# Other ways to move main.
check deny  Bash  "$G update-ref refs/heads/main HEAD~1"
check deny  Bash  "$G stash"
check deny  Bash  "$G stash pop"
check allow Bash  "$G stash list"
check deny  Bash  "$G branch -f main HEAD~1"
check deny  Bash  "$G branch -D main"
check deny  Bash  "$G pull"
check deny  Bash  "$G pull origin main"
check allow Bash  "$G pull --ff-only"
# Writing into the checkout from Bash is editing it.
check deny  Bash  "echo x > README.md"
check deny  Bash  "echo x >> $W/r/README.md"
check deny  Bash  "echo x 2> README.md"
check deny  Bash  "echo x &> README.md"
check deny  Bash  "echo x | tee -a README.md"
check deny  Bash  "echo x > newdir/sub/file.txt"
check allow Bash  "echo x > /dev/null 2>&1"
check allow Bash  "echo x > $W/outside.txt"
check allow Bash  "echo \"a > README.md\""
check allow Bash  "$G log > /tmp/log.txt"
# A target built from a variable, ~ or $(...) is not a file in the checkout.
check allow Bash  "S=$W; echo x > \$S/out.txt"
check allow Bash  "echo x | tee \"\$TMPDIR/out.txt\""
check allow Bash  "echo x > ~/out.txt"
check allow Bash  "echo x > \$(mktemp)"
check allow Bash  "cd \$S && echo x > out.txt"

git switch -qc fix/1-test
echo "== on fix/1-test"
check allow Write "$W/r/README.md"
check allow Bash  "$G checkout README.md && $G $C -m x"
check deny  Bash  "$G checkout main && $G $C -m x"
check deny  Bash  "$G push origin main"

# ── Detached HEAD during a rebase (#97) ─────────────────────────
# A conflicted rebase is judged by the branch being rebased, not by the
# detached HEAD it leaves behind, or it could never be finished or aborted.
conflict() { # branch-to-rebase onto
  git switch -q main && printf 'ours\n' > conflict.txt && git add conflict.txt && git commit -qm ours
  git switch -qc "$1" main~1 && printf 'theirs\n' > conflict.txt && git add conflict.txt && git commit -qm theirs
  git switch -q "$1" && git rebase -q "$2" >/dev/null 2>&1
  rebasing "$PWD"
}

# Without this the rebase cases would still pass if the rebase never stopped.
rebasing() {
  git -C "$1" symbolic-ref --quiet HEAD >/dev/null \
    && { echo "FAIL  no conflicted rebase in $1: HEAD is not detached"; fail=1; }
}

echo "== rebase of an issue branch stopped on a conflict"
conflict fix/2-rebase main
check allow Bash  "$G add -A"
check allow Bash  "$G rebase --continue"
check allow Bash  "$G rebase --abort"
check allow Write "$W/r/conflict.txt"
git rebase --abort >/dev/null 2>&1

echo "== rebase of main stopped on a conflict"
git switch -q main && git rebase -q fix/2-rebase >/dev/null 2>&1
rebasing "$W/r"
check deny  Bash  "$G add -A"
check deny  Write "$W/r/conflict.txt"
git rebase --abort >/dev/null 2>&1
git switch -q fix/1-test

printf '#!/bin/sh\necho stub ok\n' > run-tests.sh; chmod +x run-tests.sh
check allow Bash  "$G push -u origin HEAD"
git branch -q --set-upstream-to=origin/main fix/1-test
git config push.default upstream
check deny  Bash  "$G push"
git config --unset push.default
git branch -q --unset-upstream fix/1-test
printf '#!/bin/sh\necho "Failed: ScheduleEngine window boundary"; exit 1\n' > run-tests.sh
check deny  Bash  "$G push -u origin HEAD"
# A push quoted in a commit message is not a push: no tests run.
check allow Bash  "$G $C -m \"$G push origin main\""

true

# ── Worktrees (#27) ──────────────────────────────────────
W2=$(mktemp -d)
trap 'rm -rf "$W" "$W2"' EXIT
git clone -q "$SRC" "$W2/r" && { cd "$W2/r" || exit 1; }
fixture_main && cp -r "$SRC/.claude" . && cp "$SRC/run-tests.sh" . && git add -A && git commit -qm fixture
export CLAUDE_PROJECT_DIR="$W2/r"
G="git"; C="commit"

# The worktree is a second checkout of the same repo, on a valid issue branch.
WT="$W2/r/.claude/worktrees/agent-x"
git worktree add -q -b fix/99-agent-work "$WT" >/dev/null 2>&1
printf '#!/bin/sh\necho worktree tests ok\n' > "$WT/run-tests.sh"; chmod +x "$WT/run-tests.sh"
printf '#!/bin/sh\necho MAIN CHECKOUT TESTS RAN; exit 1\n' > "$W2/r/run-tests.sh"; chmod +x "$W2/r/run-tests.sh"

check() { # expected tool arg cwd
  local exp="$1" tool="$2" arg="$3" cwd="$4" got
  got=$(jq -nc --arg t "$tool" --arg a "$arg" --arg cwd "$cwd" \
    'if $t=="Bash" then {tool_name:$t,cwd:$cwd,tool_input:{command:$a}} else {tool_name:$t,cwd:$cwd,tool_input:{file_path:$a}} end' \
    | "$W2/r/.claude/hooks/branch-guard.sh" | jq -r '.hookSpecificOutput.permissionDecision // empty')
  got=${got:-allow}
  if [[ "$got" == "$exp" ]]; then echo "ok    $exp  ${arg//$'\n'/ } [cwd=${cwd##*/}]"
  else echo "FAIL  want $exp got $got  ${arg//$'\n'/ } [cwd=${cwd##*/}]"; fail=1; fi
}

echo "== worktree on issue branch, main checkout on main"
check allow Write "$WT/README.md" "$WT"
check deny  Write "$W2/r/README.md" "$W2/r"
check allow Bash  "$G add -A" "$WT"
check deny  Bash  "$G add -A" "$W2/r"
check allow Bash  "$G $C -m x" "$WT"
check deny  Bash  "$G push origin main" "$WT"
# The gate must run the worktree's own script: the main checkout's copy fails.
check allow Bash  "$G push -u origin HEAD" "$WT"

echo "== cd moves the checkout a command is judged against"
check deny  Bash  "cd $W2/r && $G $C -m x" "$WT"
check allow Bash  "cd $WT && $G $C -m x" "$W2/r"
check deny  Bash  "cd $WT && cd - && $G $C -m x" "$W2/r"
check deny  Bash  "(cd $WT) && $G $C -m x" "$W2/r"
check allow Bash  "(cd $WT && $G $C -m x)" "$W2/r"
check deny  Bash  "pushd $WT && popd && $G $C -m x" "$W2/r"
check deny  Bash  "env -C $W2/r $G $C -m x" "$WT"
check deny  Bash  "cd $W2/r && echo x > README.md" "$WT"
check allow Bash  "cd $WT && echo x > README.md" "$W2/r"
check deny  Bash  "cd /tmp && $G -C $W2/r $C -m x" "$WT"
check allow Bash  "cd /tmp && $G $C -m x" "$W2/r"

echo "== rebase in a worktree reads that worktree's head-name"
printf 'ours\n' > "$W2/r/conflict.txt"
git -C "$W2/r" add conflict.txt run-tests.sh && git -C "$W2/r" commit -qm ours
printf 'theirs\n' > "$WT/conflict.txt"
git -C "$WT" add conflict.txt run-tests.sh && git -C "$WT" commit -qm theirs
git -C "$WT" rebase -q main >/dev/null 2>&1
git -C "$WT" symbolic-ref --quiet HEAD >/dev/null \
  && { echo "FAIL  no conflicted rebase in the worktree: HEAD is not detached"; fail=1; }
check allow Bash  "$G add -A" "$WT"
check allow Bash  "$G -C $WT add -A" "$W2/r"
check deny  Bash  "$G add -A" "$W2/r"
git -C "$WT" rebase --abort >/dev/null 2>&1

echo "== worktree switched to main"
git -C "$WT" switch -q main 2>/dev/null || git -C "$WT" checkout -q --detach
check deny Bash "$G $C -m x" "$WT"

echo "== -C into a nested worktree is judged by that worktree"
git -C "$WT" switch -q fix/99-agent-work
check allow Bash "$G -C $WT $C -m x" "$W2/r"
check allow Bash "$G -C $WT add -A" "$W2/r"
git -C "$WT" switch -q main 2>/dev/null || git -C "$WT" checkout -q --detach
check deny  Bash "$G -C $WT $C -m x" "$W2/r"

echo "== unrelated repo is none of our business"
OTHER="$W2/other"; mkdir -p "$OTHER" && git -C "$OTHER" init -q && git -C "$OTHER" commit -q --allow-empty -m init
check allow Write "$OTHER/file.txt" "$OTHER"
check allow Bash  "$G $C -m x" "$OTHER"

git -C "$W2/r" worktree remove --force "$WT" >/dev/null 2>&1

if (( fail )); then
  echo "❌ branch-guard tests failed"
  exit 1
fi
echo "✅ branch-guard tests passed"
