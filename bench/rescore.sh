#!/usr/bin/env bash
# Recomputes result.json for every run in a results directory from its logs.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
dir=$(cd "$1" && pwd)
for r in "$dir"/*/result.json; do
  read -r task agent rep secs aexit pass < <(python3 -c "import json,sys;d=json.load(open(sys.argv[1]));print(d['task'],d['agent'],d['rep'],d['secs'],d['agent_exit'],int(d['pass']))" "$r")
  python3 "$root/bench/score_run.py" "$(dirname "$r")" "$task" "$agent" "$rep" "$secs" "$aexit" "$((1 - pass))" >/dev/null
done
python3 "$root/bench/summarize.py" "$dir"
