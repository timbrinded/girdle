#!/usr/bin/env bash
# The checks are called through check, which shellcheck can't follow.
# shellcheck disable=SC2317,SC2329
# End-to-end checks of a girdle binary against a real LLM, and Jev when
# TYPESAFE_API_KEY is set: headless runs (-p), and the TUI driven through
# tmux. CI runs it on every change and around every release.
#
#   scripts/e2e.sh path/to/girdle
#
# Needs OPENROUTER_API_KEY, go, git, jq and tmux. GIRDLE_MODEL picks the model
# (default: Girdle's own default). Everything runs in a temporary directory
# with its own config, cache and state, so local settings play no part.
set -uo pipefail

bin=$(realpath "${1:?usage: scripts/e2e.sh path/to/girdle}")
[[ -n ${OPENROUTER_API_KEY:-} ]] || { echo "e2e: OPENROUTER_API_KEY is not set" >&2; exit 1; }

work=$(mktemp -d)
sock=girdle-e2e-$$
cleanup() {
	tmux -L "$sock" kill-server 2>/dev/null
	rm -rf "$work"
}
trap cleanup EXIT
export XDG_CONFIG_HOME=$work/config XDG_CACHE_HOME=$work/cache XDG_STATE_HOME=$work/state
export GIRDLE_NO_UPDATE_CHECK=1

flags=()
if [[ -z ${TYPESAFE_API_KEY:-} ]]; then
	echo "e2e: TYPESAFE_API_KEY is not set, so these runs go without Jev" >&2
	flags+=(-no-checkpoints)
fi

failed=0
check() {
	local name=$1
	shift
	local start=$SECONDS
	if "$@"; then
		echo "ok   $name ($((SECONDS - start))s)"
	else
		echo "FAIL $name ($((SECONDS - start))s)"
		failed=1
	fi
}

# fixture makes a small Go project under git.
fixture() {
	local dir=$work/$1
	mkdir -p "$dir"
	cat >"$dir/go.mod" <<'EOF'
module fixture

go 1.22
EOF
	cat >"$dir/main.go" <<'EOF'
package main

import "fmt"

func main() { fmt.Println(greet("world")) }

func greet(name string) string { return "hello " + name }
EOF
	git -C "$dir" init -q
	git -C "$dir" add -A
	git -C "$dir" -c user.name=e2e -c user.email=e2e@example.com commit -qm init
	echo "$dir"
}

# headless runs one request with -p in dir. Its output goes to $work/out,
# and the outcomes it accepts are exit codes 0 (done) and 2 (over to you).
headless() {
	local dir=$1 prompt=$2 limit=${3:-150}
	timeout $((limit + 30)) "$bin" -C "$dir" -timeout "${limit}s" ${flags[@]+"${flags[@]}"} -p "$prompt" >"$work/out" 2>&1
	local code=$?
	if [[ $code != 0 && $code != 2 ]]; then
		echo "  exit code $code; output ends:" >&2
		tail -20 "$work/out" | sed 's/^/  | /' >&2
		return 1
	fi
}

# pane waits up to secs for the tmux session's screen to match pattern.
pane() {
	local session=$1 pattern=$2 secs=$3
	for ((i = 0; i < secs * 2; i++)); do
		tmux -L "$sock" capture-pane -p -t "$session" 2>/dev/null | grep -Eq "$pattern" && return 0
		sleep 0.5
	done
	echo "  the screen never matched /$pattern/; it ends:" >&2
	tmux -L "$sock" capture-pane -p -t "$session" 2>/dev/null | grep -v '^\s*$' | tail -25 | sed 's/^/  | /' >&2
	return 1
}

# tui starts the TUI in dir in a tmux session, and prints a marker when it
# exits.
tui() {
	local session=$1 dir=$2
	tmux -L "$sock" new-session -d -s "$session" -x 120 -y 40 -c "$dir" \
		"$(printf '%q ' "$bin" ${flags[@]+"${flags[@]}"}); echo GIRDLE_EXITED \$?; sleep 600"
	pane "$session" "Ask Girdle" 20
}

version() {
	"$bin" -version | grep -q '^girdle '
}

question() {
	local dir
	dir=$(fixture question)
	headless "$dir" "What does greet return for the name Tim?" && grep -q "hello Tim" "$work/out"
}

change() {
	local dir
	dir=$(fixture change)
	headless "$dir" 'Make greet return "hi " + name instead, and add a test for it.' &&
		grep -q '"hi "' "$dir/main.go" &&
		compgen -G "$dir/*_test.go" >/dev/null &&
		(cd "$dir" && go test ./... >/dev/null)
}

# /usr stands in for a home directory: not a repository, and hundreds of
# thousands of real files. v0.2.0 walked all of them before every request,
# ignoring -timeout and ctrl+c, and in a real home directory never finished.
bigdir=/usr

not_a_repo() {
	headless "$bigdir" "hello" 30
}

# Like fff, Girdle doesn't read a home directory up front, and says so.
home_dir() {
	headless "$HOME" "hello" 30 && grep -q "running in your home directory" "$work/out"
}

# Tens of thousands of tiny files fit the snapshot's size budget, but v0.2.0
# sent each with its file tags, a prompt of over a million tokens.
many_files() {
	local dir=$work/many
	for d in $(seq 1 20); do
		mkdir -p "$dir/d$d"
		(cd "$dir/d$d" && seq 1 2500 | xargs touch)
	done
	timeout 60 "$bin" -C "$dir" -timeout 30s -json ${flags[@]+"${flags[@]}"} -p "hello" >"$work/events" 2>&1
	local tokens
	tokens=$(jq -s '[.[] | select(.type == "step") | .usage.input_tokens // 0] | max // 0' "$work/events" 2>/dev/null)
	if [[ -z $tokens || $tokens == 0 || $tokens -gt 100000 ]]; then
		echo "  first prompt: ${tokens:-unknown} input tokens; output ends:" >&2
		tail -5 "$work/events" | cut -c1-300 | sed 's/^/  | /' >&2
		return 1
	fi
}

tui_question() {
	local dir
	dir=$(fixture tui)
	tui question "$dir" || return 1
	tmux -L "$sock" send-keys -t question "What does greet return for the name Tim?" Enter
	pane question "done ·|over to you ·" 150 &&
		pane question "hello Tim" 1 &&
		tmux -L "$sock" send-keys -t question C-c &&
		pane question "GIRDLE_EXITED 0" 10
}

# In the large directory, ctrl+c stops a running request, and a second
# quits. v0.2.0 never got past its snapshot there, and ignored ctrl+c.
tui_ctrl_c() {
	tui ctrlc "$bigdir" || return 1
	tmux -L "$sock" send-keys -t ctrlc "Write a 3000-word essay on the history of belts." Enter
	pane ctrlc "is working" 10 || return 1
	sleep 2
	tmux -L "$sock" send-keys -t ctrlc C-c
	pane ctrlc "stopped ·" 15 || return 1
	tmux -L "$sock" send-keys -t ctrlc C-c
	pane ctrlc "GIRDLE_EXITED 0" 10
}

echo "e2e: $("$bin" -version)"
check "version" version
check "headless question" question
check "headless change with a test" change
check "headless in a large directory that isn't a repository" not_a_repo
check "headless in the home directory skips the snapshot and says so" home_dir
check "headless among many tiny files keeps the prompt small" many_files
check "TUI question, then ctrl+c quits" tui_question
check "TUI ctrl+c stops a request in a large directory, then quits" tui_ctrl_c
exit $failed
