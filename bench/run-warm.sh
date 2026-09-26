#!/usr/bin/env bash
# Runs one agent on one task several times in a single warm working
# directory, the way someone works in their own checkout: the directory is
# built once, its build caches are warmed by running the task's check once,
# and git resets it between runs. Its path stays the same, so compiler caches
# and prompt prefixes behave as they do in daily use.
#
#   bench/run-warm.sh <agent> <task> <reps> <outroot>
set -uo pipefail
agent=$1 task=$2 reps=$3 outroot=$4
root=$(cd "$(dirname "$0")/.." && pwd)
tdir=$root/bench/tasks/$task
[[ -d $tdir ]] || tdir=$root/bench/scale/$task
warm=$root/bench/.warm/$agent/$task
if [[ ! -d $warm/repo/.git ]]; then
  rm -rf "$warm"
  mkdir -p "$warm"
  "$root/bench/prepare.sh" "$tdir" "$warm/repo"
  (cd "$warm/repo" && TASK_DIR="$tdir" bash "$tdir/check.sh" >/dev/null 2>&1)
fi
for rep in $(seq 1 "$reps"); do
  WARM_DIR=$warm/repo "$root/bench/run-one.sh" "$agent" "$task" "$rep" "$outroot"
done
