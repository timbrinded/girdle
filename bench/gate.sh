#!/usr/bin/env bash
# Segment 1 gate: two scenarios against real Jev and a real LLM.
#   1. rename-across-files: Girdle finishes a rename unattended, tests pass,
#      and the event log records each turn-end decision.
#   2. announce-then-stop: the conversation starts with the LLM announcing an
#      action and ending its turn. Jev must nudge, and the run must complete.
set -uo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
for k in OPENROUTER_API_KEY TYPESAFE_API_KEY; do
  [[ -n ${!k:-} ]] || export "$k=$(zsh -ic "printf %s \"\$$k\"" 2>/dev/null)"
done
out=$root/bench/results/gate-$(date +%Y%m%d-%H%M%S)
mkdir -p "$out"
girdle=$out/girdle
(cd "$root" && go build -o "$girdle" ./cmd/girdle) || exit 1
task=$root/bench/tasks/go-rename
status=0

fresh() {
  local w
  w=$(mktemp -d)/repo
  cp -R "$task/repo" "$w"
  (cd "$w" && git init -q && git add -A && git -c user.email=gate@girdle -c user.name=gate commit -qm init)
  echo "$w"
}

decisions() { grep -c '"type":"decision"' "$1"; }
first_action() { grep '"type":"decision"' "$1" | head -1 | python3 -c 'import json,sys; print(json.loads(sys.stdin.readline())["decision"]["action"])'; }
outcome() { grep '"type":"run_end"' "$1" | tail -1 | python3 -c 'import json,sys; print(json.loads(sys.stdin.readline())["outcome"])'; }

# 1. rename-across-files
w=$(fresh)
"$girdle" -C "$w" -p "$(cat "$task/task.md")" -log "$out/rename.jsonl" >"$out/rename.txt" 2>&1 </dev/null
if (cd "$w" && TASK_DIR=$task bash "$task/check.sh") >"$out/rename.check" 2>&1 &&
  [[ $(outcome "$out/rename.jsonl") == done && $(decisions "$out/rename.jsonl") -ge 1 ]]; then
  echo "✓ rename-across-files ($(decisions "$out/rename.jsonl") decision(s))"
else
  echo "✗ rename-across-files: outcome=$(outcome "$out/rename.jsonl"); see $out/rename.*"
  status=1
fi

# 2. announce-then-stop
w=$(fresh)
"$girdle" -C "$w" -seed "$root/bench/scenarios/announce-then-stop.json" -log "$out/announce.jsonl" >"$out/announce.txt" 2>&1 </dev/null
if (cd "$w" && TASK_DIR=$task bash "$task/check.sh") >"$out/announce.check" 2>&1 &&
  [[ $(first_action "$out/announce.jsonl") == nudge && $(outcome "$out/announce.jsonl") == done ]]; then
  echo "✓ announce-then-stop (nudged, then finished)"
else
  echo "✗ announce-then-stop: first action=$(first_action "$out/announce.jsonl") outcome=$(outcome "$out/announce.jsonl"); see $out/announce.*"
  status=1
fi
exit $status
