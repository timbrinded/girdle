# Girdle

An agent you can leave alone: a terminal coding agent that keeps working when it should, stops exactly when it needs you, and never claims to be done when it isn't.

The LLM decides and does the work. [Jev](https://docs.typesafe.ai), a calibrated decision model, judges meaning at checkpoints around every step. Code owns the loop.

## Try it

Girdle needs `OPENROUTER_API_KEY` and `TYPESAFE_API_KEY` in the environment, and works best with [ast-grep](https://ast-grep.github.io) installed (`brew install ast-grep`). Girdle uses it to parse code and shell commands.

```bash
go build -o bin/girdle ./cmd/girdle
bin/girdle                          # TUI in the current directory
bin/girdle -p "fix the failing test" # headless: exits when done (0), or when it needs you (2)
```

Useful flags: `-C dir`, `-model`, `-reasoning`, `-json` (headless events as JSON lines), `-no-checkpoints` (turn Jev off), `-log path`.

Every shell command passes a tripwire first. ast-grep parses the command, and code blocks what can never be allowed: recursive deletes outside the project, force-pushes to a shared branch, and secrets sent off the machine. Commands that delete, push and send nothing run at once. Jev judges the rest. A blocked command hands the run back to you. `-tripwire=false` turns it off. See [decision 0014](docs/decisions/0014-ast-grep-with-jev.md).

`-fast` turns on the fast flow, which finishes most tasks in one LLM step. The repository's files go out with the request. One `apply` call makes every change and runs a check. For a large repository, Jev also picks the other files the request will need, one isolated question per file, and they go out too. Jev ends the run as soon as the check's result shows the task done, reading the changes and the check's output with a broad set of questions. When a request renames or removes something, Girdle doesn't stop while the old name still appears where it must change. Each LLM call is raced three times, and the first call starts before routing finishes. Alongside the main call, a second call writes an independent test from the task's words, and Girdle runs it once the agent's own check passes. In warm, daily-use conditions it is twice as fast as the default flow on both benchmark suites, at the same pass rate. See [research/06](research/06-optimisation-stage-summary.md) for the summary, and decisions [0005](docs/decisions/0005-fast-flow.md) to [0013](docs/decisions/0013-jev-fan-out.md) for the detail. Any part of `-fast` can be left out by setting its flag to false, for example `-fast -crosscheck=false`.

Every session writes an event log, including each Jev decision, to `~/.local/state/girdle/sessions/`.

## Benchmark

`bench/` runs Girdle and vanilla Pi on the same tasks with the same model and scores them with hidden tests:

```bash
bench/validate.sh                 # every task fails untouched and passes its reference solution
bench/gate.sh                     # Segment 1 gate scenarios
bench/bench.sh -r 3 -j 4          # Segment 1 comparison: girdle, girdle-nojev, pi

# A new idea: the fast flow against the fast flow with the idea, warm, on the tasks it targets.
# Any Girdle flag goes after a +, so girdle-fast+crosscheck=false is an ablation.
bench/bench.sh -w -t "gm-strike-tag py-config" -a "girdle-fast girdle-fast+heartbeat=false" -r 3

# Milestones only: both suites, fast flow against the default flow.
bench/bench.sh -w -a "girdle-fast girdle" -r 3
bench/bench.sh -w -s scale -a "girdle-fast girdle" -r 3
```

`-w` runs warm: one reused, warmed directory per agent and task, as in daily use. `-s scale` picks the six tasks on real repositories (goldmark, more-itertools).

The scale suite's tasks name an upstream repository and commit in a `source` file. `bench/prepare.sh` clones each one once into `bench/.cache`, so their code is never vendored here.

Early and private. See [CLAUDE.md](CLAUDE.md) for the North Star and how we work, and [research/](research/) for the background.

Licensed under Apache-2.0.
