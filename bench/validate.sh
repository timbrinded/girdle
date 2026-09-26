#!/usr/bin/env bash
# Checks every task's check.sh: it must fail on the untouched repo and pass
# once the reference solution is applied.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
status=0
for tdir in "$root"/bench/tasks/*/; do
  task=$(basename "$tdir")
  work=$(mktemp -d)
  cp -R "$tdir/repo/." "$work"
  if (cd "$work" && TASK_DIR="$tdir" bash "$tdir/check.sh") >/dev/null 2>&1; then
    echo "✗ $task: check passes on the untouched repo"; status=1
  else
    rm -rf "$work"; work=$(mktemp -d)
    cp -R "$tdir/repo/." "$work"; cp -R "$tdir/solution/." "$work"
    if out=$(cd "$work" && TASK_DIR="$tdir" bash "$tdir/check.sh" 2>&1); then
      echo "✓ $task"
    else
      echo "✗ $task: check fails on the reference solution"; echo "$out" | tail -15; status=1
    fi
  fi
  rm -rf "$work"
done
exit $status
