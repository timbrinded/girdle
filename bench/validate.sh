#!/usr/bin/env bash
# Checks every task's check.sh: it must fail on the starting repo and pass
# once the reference solution (solution/ or solution.patch) is applied.
#
#   bench/validate.sh [task-dir...]    (default: every task in both suites)
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
status=0
dirs=("$@")
[[ ${#dirs[@]} -gt 0 ]] || dirs=("$root"/bench/tasks/*/ "$root"/bench/scale/*/ "$root"/bench/hard/*/ "$root"/bench/holdout/*/)
for tdir in "${dirs[@]}"; do
  [[ -d $tdir ]] || continue
  tdir=$(cd "$tdir" && pwd)
  task=$(basename "$tdir")
  work=$(mktemp -d)/repo
  "$root/bench/prepare.sh" "$tdir" "$work"
  if (cd "$work" && TASK_DIR="$tdir" bash "$tdir/check.sh") >/dev/null 2>&1; then
    echo "✗ $task: check passes on the starting repo"; status=1
  else
    rm -rf "$(dirname "$work")"; work=$(mktemp -d)/repo
    "$root/bench/prepare.sh" "$tdir" "$work"
    if [[ -f $tdir/solution.patch ]]; then
      (cd "$work" && git apply "$tdir/solution.patch")
    else
      cp -R "$tdir/solution/." "$work"
    fi
    if out=$(cd "$work" && TASK_DIR="$tdir" bash "$tdir/check.sh" 2>&1); then
      echo "✓ $task"
    else
      echo "✗ $task: check fails on the reference solution"; echo "$out" | tail -15; status=1
    fi
  fi
  rm -rf "$(dirname "$work")"
done
exit $status
