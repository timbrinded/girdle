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

Every session writes an event log, including each Jev decision, to `~/.local/state/girdle/sessions/`.

## Benchmark

`bench/` runs Girdle and vanilla Pi on the same tasks with the same model and scores them with hidden tests:

```bash
bench/validate.sh                 # every task fails untouched and passes its reference solution
bench/gate.sh                     # Segment 1 gate scenarios
bench/bench.sh -r 3 -j 4          # full comparison: girdle, girdle-nojev, pi
```

Early and private. See [CLAUDE.md](CLAUDE.md) for the North Star and how we work, and [research/](research/) for the background.

Licensed under Apache-2.0.
