#!/usr/bin/env bash
# Runs one agent on one task in a fresh copy of the task's repo, then scores
# it with the task's check.sh.
#
#   bench/run-one.sh <agent> <task> <rep> <outroot>
#
# Agents: girdle, girdle-nojev (checkpoints off), girdle-fast (-fast flow),
# pi (vanilla Pi, run in tmux). A Girdle agent can carry Girdle flags after
# a +, which is how an idea or an ablation is tested:
# girdle-fast+reproduce=false runs -fast -reproduce=false, and
# girdle-fast+race=5+hedge=2s runs -fast -race=5 -hedge=2s.
# Expects OPENROUTER_API_KEY and TYPESAFE_API_KEY in the environment.
set -uo pipefail

agent=$1 task=$2 rep=$3 outroot=$4
root=$(cd "$(dirname "$0")/.." && pwd)
tdir=$root/bench/tasks/$task
[[ -d $tdir ]] || tdir=$root/bench/scale/$task
[[ -d $tdir ]] || tdir=$root/bench/hard/$task
[[ -d $tdir ]] || tdir=$root/bench/holdout/$task
# BENCH_PROVIDER picks Girdle's provider: openrouter (default) or zen,
# OpenCode Zen. BENCH_MODEL defaults to that provider's default model.
provider=${BENCH_PROVIDER:-openrouter}
# Space Bunny Alpha is free on OpenRouter for now (2026-09-27); it is an
# anonymous model whose provider may log prompts, so public repos only.
default_model=stealth/space-bunny-alpha
[[ $provider == zen ]] && default_model=longcat-2.5-preview-free
model=${BENCH_MODEL:-$default_model}
# Girdle's shell commands run offline (-offline-tools), so an agent can't
# fetch the upstream fix a task was built from (decision 0018).
# BENCH_JEV picks where Girdle calls Jev: typesafe (pinned) or zen. Zen's
# free jev-1.13-free matches the pinned model, but its quota ran out under
# benchmark load within minutes (decision 0017), so it is opt-in.
jev_via=${BENCH_JEV:-typesafe}
reasoning=${BENCH_REASONING:-medium}
# Hard tasks are long by design: they get 20 minutes unless BENCH_TIMEOUT says otherwise.
default_timeout=600
[[ $tdir == */bench/hard/* || $tdir == */bench/holdout/* ]] && default_timeout=1200
timeout_s=${BENCH_TIMEOUT:-$default_timeout}
mkdir -p "$outroot"
outroot=$(cd "$outroot" && pwd)
id=$task.$agent.$rep
out=$outroot/$id
mkdir -p "$out"

base=${agent%%+*}
flags=()
if [[ $agent == *+* ]]; then
  IFS=+ read -ra parts <<<"${agent#*+}"
  for f in "${parts[@]}"; do flags+=("-$f"); done
fi
if [[ $base == pi ]] && ((${#flags[@]})); then
  echo "pi takes no + flags: $agent" >&2
  exit 2
fi

# In warm mode (WARM_DIR, from run-warm.sh) the working directory is reused
# and reset with git; otherwise each run gets a fresh copy.
if [[ -n ${WARM_DIR:-} ]]; then
  work=$WARM_DIR
  (cd "$work" && git reset -q --hard && git clean -fdq)
else
  work=$(mktemp -d)/repo
  "$root/bench/prepare.sh" "$tdir" "$work"
fi
prompt=$(cat "$tdir/task.md")

# Other copies of the code under test would let an agent copy the fix: the
# benchmark's clones of other commits, other runs' warm copies, and a
# released version in the Go module cache or in Python's site-packages. An
# agent read gjson's released feature from the module cache (decision 0019),
# so Girdle's tools may not read any of them; the working copy stays open.
deny=(-deny-read "$root/bench/.cache" -deny-read "$root/bench/.warm")
if [[ -f $work/go.mod ]]; then
  mod=$(awk '$1 == "module" {print $2; exit}' "$work/go.mod")
  modcache=$(go env GOMODCACHE)
  # The module cache writes a capital as ! and the lower-case letter. A
  # major-version suffix is dropped too, to cover the other majors.
  for m in "$mod" "${mod%/v[0-9]*}"; do
    esc=$(printf %s "$m" | perl -pe 's/([A-Z])/!\l$1/g')
    deny+=(-deny-read "$modcache/$esc@" -deny-read "$modcache/cache/download/$esc/")
  done
fi
for pkg in "$work"/*/__init__.py "$work"/src/*/__init__.py; do
  [[ -f $pkg ]] || continue
  name=$(basename "$(dirname "$pkg")")
  while read -r site; do
    [[ -n $site ]] && deny+=(-deny-read "$site/$name")
  done < <(python3 -c 'import site; print("\n".join(site.getsitepackages() + [site.getusersitepackages()]))' 2>/dev/null)
done

start=$(date +%s)
case $base in
girdle | girdle-nojev | girdle-fast)
  extra=("${deny[@]}")
  [[ $base == girdle-nojev ]] && extra+=(--no-checkpoints)
  [[ $base == girdle-fast ]] && extra+=(--fast)
  extra+=(${flags[@]+"${flags[@]}"})
  timeout "$timeout_s" "$root/bin/girdle" -C "$work" -p "$prompt" -json \
    -provider "$provider" -model "$model" -jev "$jev_via" -reasoning "$reasoning" -offline-tools -log "$out/events.jsonl" ${extra[@]+"${extra[@]}"} \
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

SCORE_MODEL="$model" SCORE_JEV="$jev_via" python3 "$root/bench/score_run.py" "$out" "$task" "$agent" "$rep" "$secs" "$agent_exit" "$check_exit"
if [[ -n ${WARM_DIR:-} ]]; then
  (cd "$work" && git reset -q --hard && git clean -fdq)
else
  rm -rf "$(dirname "$work")"
fi
