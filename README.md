# Girdle

An agent you can leave alone: a terminal coding agent that keeps working when it should, stops exactly when it needs you, and never claims to be done when it isn't.

The LLM decides and does the work. [Jev](https://docs.typesafe.ai), a calibrated decision model, judges meaning at checkpoints around every step. Code owns the loop.

## Try it

Girdle needs `OPENROUTER_API_KEY` and `TYPESAFE_API_KEY` in the environment.

```bash
go build -o bin/girdle ./cmd/girdle
bin/girdle                          # TUI in the current directory
bin/girdle -p "fix the failing test" # headless: exits when done (0), or when it needs you (2)
```

Useful flags: `-C dir`, `-model`, `-reasoning`, `-json` (headless events as JSON lines), `-no-checkpoints` (turn Jev off), `-log path`.

`-fast` turns on the fast flow, which finishes most tasks in one LLM step. The repository's files go out with the request. One `apply` call makes every change and runs a check. Jev ends the run as soon as the check's result shows the task done. Each LLM call is raced three times, and the first call starts before routing finishes. Alongside the main call, a second call writes an independent test from the task's words, and Girdle runs it once the agent's own check passes. See decisions [0005](docs/decisions/0005-fast-flow.md) to [0009](docs/decisions/0009-cross-check.md), and [research/05](research/05-fast-flow-at-scale.md) for results.

Every session writes an event log, including each Jev decision, to `~/.local/state/girdle/sessions/`.

## Benchmark

`bench/` runs Girdle and vanilla Pi on the same tasks with the same model and scores them with hidden tests:

```bash
bench/validate.sh                 # every task fails untouched and passes its reference solution
bench/gate.sh                     # Segment 1 gate scenarios
bench/bench.sh -r 3 -j 4          # full comparison: girdle, girdle-nojev, pi
bench/bench.sh -a "girdle-fast girdle-fast-r1-low" -r 3   # fast flow; -r<N> sets racing, -low fixes the effort
bench/bench.sh -s scale -a "girdle-fast girdle" -r 3       # six tasks on real repositories (goldmark, more-itertools)
```

The scale suite's tasks name an upstream repository and commit in a `source` file. `bench/prepare.sh` clones each one once into `bench/.cache`, so their code is never vendored here.

Early and private. See [CLAUDE.md](CLAUDE.md) for the North Star and how we work, and [research/](research/) for the background.

Licensed under Apache-2.0.
