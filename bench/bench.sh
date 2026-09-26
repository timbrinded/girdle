#!/usr/bin/env bash
# Runs the benchmark: every task × agent × rep, in parallel, then summarises.
#
#   bench/bench.sh [-a "girdle pi"] [-t "go-rename py-config"] [-r 3] [-j 4] [-o outdir]
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
agents="girdle girdle-nojev pi"
tasks=$(cd "$root/bench/tasks" && printf '%s ' */ | tr -d /)
reps=3
jobs=4
outroot=$root/bench/results/$(date +%Y%m%d-%H%M%S)
while getopts "a:t:r:j:o:" opt; do
  case $opt in
  a) agents=$OPTARG ;;
  t) tasks=$OPTARG ;;
  r) reps=$OPTARG ;;
  j) jobs=$OPTARG ;;
  o) outroot=$OPTARG ;;
  *) exit 2 ;;
  esac
done

# Keys live in the interactive shell profile; load them without printing.
for k in OPENROUTER_API_KEY TYPESAFE_API_KEY; do
  if [[ -z ${!k:-} ]]; then
    v=$(zsh -ic "printf %s \"\$$k\"" 2>/dev/null)
    export "$k=$v"
  fi
done

(cd "$root" && go build -o bin/girdle ./cmd/girdle) || exit 1
mkdir -p "$outroot"
outroot=$(cd "$outroot" && pwd)
echo "results: $outroot"

for rep in $(seq 1 "$reps"); do
  for task in $tasks; do
    for agent in $agents; do
      echo "$agent $task $rep"
    done
  done
done | xargs -P "$jobs" -L 1 bash -c '"'"$root"'/bench/run-one.sh" "$0" "$1" "$2" "'"$outroot"'"'

python3 "$root/bench/summarize.py" "$outroot"
