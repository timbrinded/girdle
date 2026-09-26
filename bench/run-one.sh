#!/usr/bin/env bash
# Runs one agent on one task in a fresh copy of the task's repo, then scores
# it with the task's check.sh.
#
#   bench/run-one.sh <agent> <task> <rep> <outroot>
#
# Agents: girdle, girdle-nojev (checkpoints off), pi (vanilla Pi, run in tmux).
# Expects OPENROUTER_API_KEY and TYPESAFE_API_KEY in the environment.
set -uo pipefail

agent=$1 task=$2 rep=$3 outroot=$4
root=$(cd "$(dirname "$0")/.." && pwd)
tdir=$root/bench/tasks/$task
model=${BENCH_MODEL:-meta/muse-spark-1.3-contributor}
reasoning=${BENCH_REASONING:-medium}
timeout_s=${BENCH_TIMEOUT:-600}
mkdir -p "$outroot"
outroot=$(cd "$outroot" && pwd)
id=$task.$agent.$rep
out=$outroot/$id
mkdir -p "$out"

work=$(mktemp -d)/repo
cp -R "$tdir/repo" "$work"
(cd "$work" && git init -q && git add -A && git -c user.email=bench@girdle -c user.name=bench commit -qm init)
prompt=$(cat "$tdir/task.md")

# An agent name ending in -low runs at low reasoning effort; for Girdle it
# also turns routing off so the effort stays fixed.
base=$agent
if [[ $agent == *-low ]]; then
  base=${agent%-low}
  reasoning=low
fi

start=$(date +%s)
case $base in
girdle | girdle-nojev)
  extra=()
  [[ $base == girdle-nojev ]] && extra+=(--no-checkpoints)
  [[ $agent == *-low ]] && extra+=(--no-route)
  timeout "$timeout_s" "$root/bin/girdle" -C "$work" -p "$prompt" -json \
    -model "$model" -reasoning "$reasoning" -log "$out/events.jsonl" ${extra[@]+"${extra[@]}"} \
    >"$out/stdout.jsonl" 2>"$out/stderr.txt" </dev/null
  agent_exit=$?
  ;;
pi)
  # Pi runs inside a detached tmux session. The key is read from the shell
  # profile inside the session, so it is never written to disk.
  chan=girdle-bench-$$-$RANDOM
  cat >"$out/pi.sh" <<EOF
cd "$work" || { echo "cannot enter $work" >"$out/stderr.txt"; tmux wait-for -S $chan; exit 1; }
export OPENROUTER_API_KEY="\$(zsh -ic 'printf %s "\$OPENROUTER_API_KEY"' 2>/dev/null)"
PI_CODING_AGENT_DIR="$root/bench/pi-agent" pi --provider openrouter --model "$model" --thinking "$reasoning" \
  --no-extensions --no-skills --no-prompt-templates --no-context-files --no-themes --no-session \
  --mode json -p "\$(cat "$tdir/task.md")" >"$out/pi.jsonl" 2>"$out/stderr.txt" </dev/null
echo \$? >"$out/pi.exit"
tmux wait-for -S $chan
EOF
  tmux new-session -d -s "$chan" "bash '$out/pi.sh'"
  (sleep "$timeout_s" && tmux kill-session -t "$chan" && tmux wait-for -S "$chan") >/dev/null 2>&1 &
  watchdog=$!
  tmux wait-for "$chan"
  pkill -P "$watchdog" 2>/dev/null
  kill "$watchdog" 2>/dev/null
  tmux kill-session -t "$chan" 2>/dev/null
  agent_exit=$(cat "$out/pi.exit" 2>/dev/null || echo 124)
  ;;
*)
  echo "unknown agent $agent" >&2
  exit 2
  ;;
esac
secs=$(($(date +%s) - start))

(cd "$work" && git status --porcelain >"$out/changed.txt" && git diff >"$out/diff.patch")
(cd "$work" && TASK_DIR="$tdir" bash "$tdir/check.sh") >"$out/check.txt" 2>&1
check_exit=$?

python3 "$root/bench/score_run.py" "$out" "$task" "$agent" "$rep" "$secs" "$agent_exit" "$check_exit"
rm -rf "$(dirname "$work")"
