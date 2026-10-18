#!/usr/bin/env bash
# PreToolUse on Edit|Write|NotebookEdit|Bash: all work happens on an issue
# branch named <type>/<issue>-<slug>, never on main, and nothing is pushed
# unless ./run-tests.sh passes.
#
# Bash commands are split the way a shell would split them: quoted strings are
# kept as one word, and code that runs elsewhere (bash -c, eval, $(...),
# backticks, a heredoc fed to a shell, env -S, xargs) is inspected as well.
# cd, pushd and subshells move the checkout a command is judged against, and
# output redirections and tee into a checkout count as edits of it.
#
# This is a guard rail against mistakes, not a sandbox: programs that write
# files by themselves (sed -i, cp, mv, an interpreter) and commands assembled
# from variables are not followed.
set -uo pipefail

BRANCH_RE='^(feat|fix|chore|docs|refactor|test|perf|ci|build|revert)/[0-9]+-[a-z0-9][a-z0-9._-]*$'
PROTECTED_RE='(^|/)(main|master)$'
SHELL_RE='^(sh|bash|dash|zsh|ksh|ash)$'
MAX_DEPTH=5
HINT="Work only on issue branches named <type>/<issue>-<slug> (e.g. fix/42-audio-regression). Find or create the GitHub issue first (see the gh skill), then: gh issue develop <N> --name <type>/<N>-<slug> --base main --checkout"

input="$(cat)"
tool="$(jq -r '.tool_name // empty' <<< "$input")"
project="$(realpath -m "${CLAUDE_PROJECT_DIR:-$(jq -r '.cwd // empty' <<< "$input")}")"
cwd="$(jq -r '.cwd // empty' <<< "$input")"
cwd="${cwd:-$project}"

# Worktrees are separate checkouts of this repo with their own branch and their
# own copy of run-tests.sh, so the checkout is resolved from what is being acted
# on, not from the project directory. They share a common git dir, which is what
# tells a worktree of this repo apart from an unrelated repo on disk.
# rev-parse prints the common dir relative to its own working directory, so it
# is resolved there rather than wherever this hook happens to run.
git_common_dir() { (cd "$1" 2>/dev/null && realpath -m "$(git rev-parse --git-common-dir 2>/dev/null)"); }
project_git_dir="$(git_common_dir "$project")"

checkout_for() {
  local dir="$1" top common
  top="$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)" || return 1
  common="$(git_common_dir "$dir")"
  [[ "$common" == "$project_git_dir" ]] || return 1
  printf '%s' "$top"
}

# The deepest directory of path that exists: a file written into a directory
# that does not exist yet still belongs to the checkout around it.
existing_dir() {
  local d="$1"
  while [[ ! -d "$d" && "$d" != / ]]; do d="$(dirname "$d")"; done
  printf '%s' "$d"
}

decide() {
  jq -n --arg d "$1" --arg r "$2" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: $d, permissionDecisionReason: $r}}'
  exit 0
}
deny() { decide deny "$1"; }
ask() { decide ask "$1"; }

valid() { [[ "$1" =~ $BRANCH_RE ]]; }

# A rebase that stops on a conflict detaches HEAD, so symbolic-ref yields
# nothing and every git command would be refused — including the `git add` and
# `git rebase --continue` that are the only way out. Both rebase backends record
# the branch being rebased in head-name, which rev-parse locates for a worktree
# too, where it does not sit under .git/ (#97).
current_branch() {
  local repo="$1" branch head_name d
  branch="$(git -C "$repo" symbolic-ref --quiet --short HEAD 2>/dev/null)" && { printf %s "$branch"; return; }
  for d in rebase-merge rebase-apply; do
    head_name="$(cd "$repo" 2>/dev/null && realpath -m "$(git rev-parse --git-path "$d/head-name" 2>/dev/null)")"
    [[ -f "$head_name" ]] || continue
    branch="$(<"$head_name")"
    printf %s "${branch#refs/heads/}"
    return
  done
  printf %s "(detached HEAD)"
}

# The branch each checkout is on, as far as the command seen so far goes:
# "git switch -c fix/1-x && git commit" commits on fix/1-x.
declare -A BRANCH_OF=()
branch_of() {
  [[ -n "${BRANCH_OF[$1]:-}" ]] || BRANCH_OF[$1]="$(current_branch "$1")"
  printf %s "${BRANCH_OF[$1]}"
}

# True for `git merge --ff-only <upstream>` / `git reset --hard <upstream>` where
# <upstream> is exactly the tracking branch of the checked-out branch.
syncs_with_upstream() {
  local repo="$1" branch="$2"; shift 2
  local upstream target="" mode=0
  upstream="$(git -C "$repo" rev-parse --abbrev-ref --symbolic-full-name "$branch@{upstream}" 2>/dev/null)" || return 1
  [[ -n "$upstream" ]] || return 1
  for a in "$@"; do
    case "$a" in
      --ff-only|--hard) mode=1 ;;
      -*) return 1 ;;
      *) [[ -n "$target" ]] && return 1; target="$a" ;;
    esac
  done
  (( mode )) && [[ "$target" == "$upstream" ]]
}

run_tests() {
  local repo="$1" log
  log="$(mktemp)"
  # run-tests.sh gives each checkout its own stack, so gates running in
  # parallel worktrees do not collide (#95).
  if ! (cd "$repo" && ./run-tests.sh) > "$log" 2>&1; then
    local tail_out
    tail_out="$(tail -n 60 "$log")"
    rm -f "$log"
    deny "Push blocked: ./run-tests.sh failed. Fix the failures, commit, and push again.
$tail_out"
  fi
  rm -f "$log"
}

# Splits a command line into simple commands, one per output line, with the
# words of each separated by \x1f. Quotes are removed but keep their content
# in one word; redirection operators come out as words of their own prefixed
# with \x02, so a quoted ">" is never mistaken for one. A subshell opens with
# the word \x03( and closes with \x03). Heredoc bodies are data and dropped,
# unless the heredoc is fed to a shell; $(...) and backticks are emitted as
# commands of their own and leave \x04 in the word they stood in.
read -r -d '' TOKENIZER <<'PERL'
use strict; use warnings;
my @out;
sub emit { my ($seg) = @_; push @out, join("\x1f", @$seg) if @$seg; @$seg = (); }
sub heredocs {
  my ($s) = @_;
  my @lines = split /\n/, $s, -1; my @keep; my @pending;
  for my $line (@lines) {
    if (@pending) {
      my ($term, $code) = @{$pending[0]};
      if ($line =~ /^\s*\Q$term\E[ \t]*$/) { shift @pending; next; }
      push @keep, $line if $code; next;
    }
    push @keep, $line;
    # A heredoc operator is "<<" outside quotes followed by a word that starts
    # with a letter or underscore: "<<X" in a string, $((1<<2)) and the here
    # string <<< open nothing, and must not hide the lines after them.
    my ($q, $i) = ('', 0);
    while ($i < length $line) {
      my $c = substr($line, $i, 1);
      if ($q) { if ($c eq '\\' && $q eq '"') { $i += 2; next; } $q = '' if $c eq $q; $i++; next; }
      if ($c eq "'" || $c eq '"') { $q = $c; $i++; next; }
      if ($c eq '\\') { $i += 2; next; }
      if (substr($line, $i, 3) eq '<<<') { $i += 3; next; }
      if (substr($line, $i) =~ /^<<-?\s*(['"]?)([A-Za-z_]\w*)\1/) {
        my ($term, $len) = ($2, length $&); my $before = substr($line, 0, $i);
        my $code = $before =~ m{(?:^|[;&|(\s])(?:\S*/)?(?:ba|da|z|k|a)?sh(?:\s+-[A-Za-z]+)*\s*$} ? 1 : 0;
        push @pending, [$term, $code]; $i += $len; next;
      }
      $i++;
    }
  }
  return join("\n", @keep);
}
sub balanced { # text after "$(" -> (inner, rest)
  my ($s) = @_; my $d = 1; my $i = 0; my $q = '';
  for (; $i < length $s; $i++) {
    my $c = substr($s, $i, 1);
    if ($q) { $q = '' if $c eq $q; next; }
    if ($c eq "'" || $c eq '"') { $q = $c; next; }
    $d++ if $c eq '('; $d-- if $c eq ')';
    return (substr($s, 0, $i), substr($s, $i + 1)) if $d == 0;
  }
  return ($s, '');
}
sub lex {
  my ($s, $depth) = @_;
  return if $depth > 5;
  my @seg; my $tok = ''; my $has = 0;
  my $end = sub { if ($has) { push @seg, $tok; } $tok = ''; $has = 0; };
  while (length $s) {
    my $c = substr($s, 0, 1);
    if ($c eq '\\') {
      my $n = substr($s, 1, 1); $s = substr($s, 2);
      if ($n ne "\n") { $tok .= $n; $has = 1; } next;
    }
    if ($c eq "'") {
      $s =~ s/^'([^']*)'?//s; (my $t = $1) =~ s/\n/ /g; $tok .= $t; $has = 1; next;
    }
    if ($c eq '"') {
      $s = substr($s, 1); $has = 1;
      while (length $s) {
        my $d = substr($s, 0, 1);
        if ($d eq '"') { $s = substr($s, 1); last; }
        if ($d eq '\\') { $tok .= substr($s, 1, 1); $s = substr($s, 2); next; }
        if ($s =~ s/^\$\(//) { my ($in, $rest) = balanced($s); $s = $rest; lex($in, $depth + 1); next; }
        if ($s =~ s/^`([^`]*)`?//s) { lex($1, $depth + 1); next; }
        $tok .= $d eq "\n" ? ' ' : $d; $s = substr($s, 1);
      }
      next;
    }
    if ($s =~ s/^\$\(//) { my ($in, $rest) = balanced($s); $s = $rest; lex($in, $depth + 1); $tok .= "\x04"; $has = 1; next; }
    if ($s =~ s/^`([^`]*)`?//s) { lex($1, $depth + 1); $tok .= "\x04"; $has = 1; next; }
    if ($c eq '#' && !$has) { $s =~ s/^[^\n]*//; next; }
    if ($s =~ s/^(&>>|&>|>>|>\||>&|>|<<<|<<-|<<|<&|<>|<)//) {
      my $op = $1;
      if ($has && $tok =~ /^\d+$/) { $op = $tok . $op; $tok = ''; $has = 0; } else { $end->(); }
      push @seg, "\x02$op"; next;
    }
    if ($s =~ s/^(&&|\|\||;;|[;&|\n])//) { $end->(); emit(\@seg); next; }
    if ($c eq '(' || $c eq ')') { $s = substr($s, 1); $end->(); emit(\@seg); push @out, "\x03$c"; next; }
    if ($c eq ' ' || $c eq "\t") { $s = substr($s, 1); $end->(); next; }
    $tok .= $c; $has = 1; $s = substr($s, 1);
  }
  $end->(); emit(\@seg);
}
local $/; my $src = <STDIN>;
lex(heredocs($src), 0);
print "$_\n" for @out;
PERL

tokenize() { perl -e "$TOKENIZER" <<< "$1"; }

resolve() { # dir path -> absolute path
  local p="$2"
  [[ "$p" == \~ || "$p" == \~/* ]] && p="${HOME:-/}${p:1}"
  if [[ "$p" == /* ]]; then realpath -m "$p"; else realpath -m "$1/$p"; fi
}

# A redirection or tee into a checkout on main is an edit of that checkout.
check_write() {
  local path="$1" r b
  [[ "$path" == /dev/* ]] && return
  # A path built from a variable or $(...) is unknown, not a file named
  # "$S/out.txt" in the current checkout (see the header).
  [[ "$path" == *'$'* || "$path" == *$'\x04'* ]] && return
  r="$(checkout_for "$(existing_dir "$(dirname "$path")")")" || return
  b="$(branch_of "$r")"
  valid "$b" || deny "Refusing to write $path on branch '$b'. $HINT"
}

pushing_repos=()

inspect() { # command-string cwd depth
  local cmd="$1" here="$2" depth="$3"
  (( depth > MAX_DEPTH )) && deny "Command nested too deeply to check."
  local -a segs stack=() dirs=()
  local oldpwd="$here" seg
  mapfile -t segs < <(tokenize "$cmd")
  for seg in "${segs[@]}"; do
    case "$seg" in
      $'\x03(') stack+=("$here"); continue ;;
      $'\x03)') (( ${#stack[@]} )) && { here="${stack[-1]}"; unset 'stack[-1]'; }; continue ;;
    esac
    local -a t=()
    IFS=$'\x1f' read -r -a t <<< "$seg"
    segment "$depth" "${t[@]}"
  done
}

# Judges one simple command. It may change "here" (cd), which inspect owns.
segment() {
  local depth="$1"; shift
  local -a t=() words=()
  local a k j from_xargs=0 seg_here="$here" git_env=0
  # Redirections first: they apply whatever the command is.
  local -a raw=("$@")
  for ((j = 0; j < ${#raw[@]}; j++)); do
    a="${raw[j]}"
    if [[ "$a" == $'\x02'* ]]; then
      local op="${a#$'\x02'}" target="${raw[j+1]:-}"
      ((j++))
      [[ "$op" == *'>'* && "$op" != *'>&' && "$target" != '&'* ]] && check_write "$(resolve "$here" "$target")"
      continue
    fi
    words+=("$a")
  done
  t=("${words[@]}")
  (( ${#t[@]} )) || return 0

  # Strip keywords, env assignments and wrappers (time git push, env -i git
  # commit, sudo -u x git ..., timeout 5 git push, xargs git ...).
  k=0
  while (( k < ${#t[@]} )); do
    a="${t[k]}"
    if [[ "$a" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      [[ "$a" =~ ^GIT_(DIR|WORK_TREE)= ]] && git_env=1
      ((k++)); continue
    fi
    case "${a##*/}" in
      if|then|else|elif|do|while|until|'!'|'{'|'}'|time|nohup|exec|builtin) ((k++)) ;;
      command)
        ((k++))
        while [[ "${t[k]:-}" == -* ]]; do [[ "${t[k]}" == -v || "${t[k]}" == -V ]] && return 0; ((k++)); done ;;
      nice|stdbuf)
        ((k++))
        while [[ "${t[k]:-}" == -* ]]; do
          [[ "${t[k]}" =~ ^-(n|i|o|e)$ ]] && ((k++))
          ((k++))
        done ;;
      timeout)
        ((k++))
        while [[ "${t[k]:-}" == -* ]]; do [[ "${t[k]}" =~ ^-(s|k)$ ]] && ((k++)); ((k++)); done
        ((k++)) ;;
      sudo|doas)
        ((k++))
        while [[ "${t[k]:-}" == -* ]]; do [[ "${t[k]}" =~ ^-[ugCDhprtU]$ ]] && ((k++)); ((k++)); done ;;
      env)
        ((k++))
        while [[ "${t[k]:-}" == -* ]]; do
          case "${t[k]}" in
            -u|--unset) ((k++)) ;;
            -C|--chdir) seg_here="$(resolve "$here" "${t[k+1]:-.}")"; ((k++)) ;;
            --chdir=*) seg_here="$(resolve "$here" "${t[k]#--chdir=}")" ;;
            -S|--split-string)
              inspect "${t[k+1]:-} ${t[*]:k+2}" "$seg_here" $((depth + 1)); return 0 ;;
            --split-string=*)
              inspect "${t[k]#--split-string=} ${t[*]:k+1}" "$seg_here" $((depth + 1)); return 0 ;;
          esac
          ((k++))
        done ;;
      xargs)
        ((k++)); from_xargs=1
        while [[ "${t[k]:-}" == -* ]]; do
          [[ "${t[k]}" =~ ^-[IinLPdEsa]$ ]] && ((k++))
          ((k++))
        done ;;
      *) break ;;
    esac
  done
  t=("${t[@]:k}")
  (( ${#t[@]} )) || return 0
  local name="${t[0]##*/}"

  if [[ "$name" =~ $SHELL_RE ]]; then
    for ((j = 1; j < ${#t[@]}; j++)); do
      [[ "${t[j]}" == -* ]] || break
      if [[ "${t[j]}" =~ ^-[A-Za-z]*c[A-Za-z]*$ ]]; then
        inspect "${t[j+1]:-}" "$seg_here" $((depth + 1))
        return 0
      fi
    done
    return 0
  fi

  case "$name" in
    eval) inspect "${t[*]:1}" "$seg_here" $((depth + 1)); return 0 ;;
    cd|pushd|popd)
      local dest=""
      for a in "${t[@]:1}"; do [[ "$a" == -[LPe@]* ]] && continue; dest="$a"; break; done
      if [[ "$name" == popd ]]; then
        (( ${#dirs[@]} )) && { dest="${dirs[-1]}"; unset 'dirs[-1]'; } || dest="$here"
      elif [[ -z "$dest" ]]; then dest="${HOME:-/}"
      elif [[ "$dest" == - ]]; then dest="$oldpwd"
      fi
      [[ "$name" == pushd ]] && dirs+=("$here")
      oldpwd="$here"
      here="$(resolve "$here" "$dest")"
      return 0 ;;
    tee)
      for a in "${t[@]:1}"; do [[ "$a" == -* ]] && continue; check_write "$(resolve "$here" "$a")"; done
      return 0 ;;
    gh)
      [[ "${t[1]:-} ${t[2]:-}" == "pr merge" ]] && ask "Merging a PR is the user's decision."
      return 0 ;;
    git) ;;
    *) return 0 ;;
  esac

  local target="$seg_here" i=1
  while [[ "${t[i]:-}" == -* ]]; do
    case "${t[i]}" in
      -C) target="$(resolve "$target" "${t[i+1]:-.}")"; ((i++)) ;;
      -c)
        [[ "${t[i+1]:-}" == alias.* ]] && deny "Defining git aliases on the command line is not allowed."
        ((i++)) ;;
      --git-dir|--work-tree|--git-dir=*|--work-tree=*) git_env=1 ;;
    esac
    ((i++))
  done
  # -C into another checkout is judged by that checkout's branch. Worktrees
  # live under .claude/worktrees inside the repo directory, so the path
  # alone does not say which checkout it belongs to — ask git.
  local repo branch
  repo="$(checkout_for "$(existing_dir "$target")")" || {
    (( git_env )) && ask "git with --git-dir/--work-tree/GIT_DIR cannot be checked; use git -C instead."
    return 0
  }
  (( git_env )) && ask "git with --git-dir/--work-tree/GIT_DIR cannot be checked; use git -C instead."
  branch="$(branch_of "$repo")"
  local sub="${t[i]:-}"
  local -a args=("${t[@]:i+1}")

  case "$sub" in
    checkout|switch)
      local created=0
      for ((j = 0; j < ${#args[@]}; j++)); do
        # A short-flag cluster counts too: -qc, -qb, --quiet -c, ...
        if [[ "${args[j]}" =~ ^-[a-zA-Z]*[bBcC]$ ]]; then
          local new="${args[j+1]:-}"
          valid "$new" || deny "Branch name '$new' is not allowed. $HINT"
          BRANCH_OF[$repo]="$new"
          created=1
        fi
      done
      if (( !created )); then
        # The branch is the first argument that names one: with "git checkout
        # -q main" only args[0] was looked at, so the flag hid the branch and
        # everything after it in the same command was judged against the
        # branch we were on before (#76).
        for a in "${args[@]}"; do
          [[ "$a" == -* ]] && continue
          git -C "$repo" show-ref --verify --quiet "refs/heads/$a" && BRANCH_OF[$repo]="$a"
          break
        done
      fi
      ;;
    branch)
      local force=0
      for a in "${args[@]}"; do [[ "$a" =~ ^-[a-zA-Z]*[fmMcCdD]$ || "$a" =~ ^--(force|move|copy|delete)$ ]] && force=1; done
      if (( force )); then
        for a in "${args[@]}"; do
          [[ "$a" != -* && "$a" =~ $PROTECTED_RE ]] && deny "Moving, renaming or deleting '$a' is not allowed. $HINT"
        done
      fi
      if [[ "${#args[@]}" -ge 1 && "${args[0]}" != -* ]]; then
        valid "${args[0]}" || deny "Branch name '${args[0]}' is not allowed. $HINT"
      fi
      ;;
    update-ref)
      for a in "${args[@]}"; do
        [[ "$a" != -* && "$a" =~ $PROTECTED_RE ]] && deny "Moving '$a' is not allowed. $HINT"
      done
      valid "$branch" || deny "Refusing 'git $sub' on branch '$branch'. $HINT"
      ;;
    stash)
      [[ "${args[0]:-}" == list || "${args[0]:-}" == show ]] && return 0
      valid "$branch" || deny "Refusing 'git $sub' on branch '$branch'. $HINT"
      ;;
    pull)
      # Only a fast-forward onto the branch's own upstream catches up.
      if [[ "${args[*]}" == --ff-only ]] && git -C "$repo" rev-parse --verify --quiet "$branch@{upstream}" >/dev/null; then
        return 0
      fi
      valid "$branch" || deny "Refusing 'git $sub' on branch '$branch'. $HINT"
      ;;
    add|mv|rm|restore|apply|commit|merge|rebase|cherry-pick|revert|reset|am)
      # Catching up with the remote is not working on main: refusing it is
      # what left main behind after every merged PR, until a release was cut
      # from stale code (#64). Only a move onto the branch's own upstream is
      # allowed, and only as a fast-forward or a reset to exactly that ref.
      if [[ "$sub" == merge || "$sub" == reset ]] && syncs_with_upstream "$repo" "$branch" "${args[@]}"; then
        return 0
      fi
      valid "$branch" || deny "Refusing 'git $sub' on branch '$branch'. $HINT"
      ;;
    push)
      (( from_xargs )) && deny "git push through xargs cannot be checked; push the issue branch directly."
      local positional=0 dst
      for a in "${args[@]}"; do
        [[ "$a" == --all || "$a" == --mirror ]] \
          && deny "Pushing to main is not allowed; push the issue branch and open a PR instead."
        [[ "$a" == -* ]] && continue
        ((positional++))
        # +main, :main, HEAD:refs/heads/main all land on main.
        dst="${a#+}"; dst="${dst##*:}"
        [[ "$dst" =~ $PROTECTED_RE ]] \
          && deny "Pushing to main is not allowed; push the issue branch and open a PR instead."
      done
      if (( positional < 2 )); then
        # No refspec: push.default decides, and a branch that tracks main
        # would push there.
        dst="$(git -C "$repo" rev-parse --abbrev-ref --symbolic-full-name "$branch@{push}" 2>/dev/null)"
        [[ "$dst" =~ $PROTECTED_RE ]] \
          && deny "Pushing to main is not allowed: '$branch' pushes to '$dst'. Push it to its own name: git push -u origin HEAD"
      fi
      valid "$branch" || deny "Refusing to push from branch '$branch'. $HINT"
      pushing_repos+=("$repo")
      ;;
  esac
  return 0
}

case "$tool" in
  Edit|Write|NotebookEdit)
    path="$(jq -r '.tool_input.file_path // .tool_input.notebook_path // empty' <<< "$input")"
    [[ -n "$path" ]] || exit 0
    path="$(realpath -m "$path")"
    # Files outside this repo (scratchpad, memory) are not project work.
    repo="$(checkout_for "$(existing_dir "$(dirname "$path")")")" || exit 0
    branch="$(current_branch "$repo")"
    valid "$branch" || deny "Refusing to edit $path on branch '$branch'. $HINT"
    ;;

  Bash)
    cmd="$(jq -r '.tool_input.command // empty' <<< "$input")"
    [[ -n "$cmd" ]] || exit 0
    here="$cwd"
    inspect "$cmd" "$cwd" 0
    if (( ${#pushing_repos[@]} )); then
      while IFS= read -r r; do run_tests "$r"; done < <(printf '%s\n' "${pushing_repos[@]}" | sort -u)
    fi
    ;;
esac
exit 0
