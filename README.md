# Girdle

An agent you can leave alone: a terminal coding agent that keeps working when it should, stops exactly when it needs you, and never claims to be done when it isn't.

The LLM decides and does the work. [Jev](https://docs.typesafe.ai), a calibrated decision model, judges meaning at checkpoints around every step. Code owns the loop.

## Try it

Girdle needs `OPENROUTER_API_KEY` and `TYPESAFE_API_KEY` in the environment. It works best with [ast-grep](https://ast-grep.github.io) installed (`brew install ast-grep`), which Girdle uses to parse code and shell commands.

```bash
go build -o bin/girdle ./cmd/girdle
bin/girdle                           # TUI in the current directory
bin/girdle -p "fix the failing test" # headless: exits when done (0), or when it needs you (2)
```

The default model is `stealth/space-bunny-alpha` on OpenRouter. It's free for now, and the fast flow is measured and tuned on it. It is an anonymous model whose provider may log prompts, so use it on public code only. It may also be withdrawn without notice. For private code, pick another with `-model` or `GIRDLE_MODEL`, for example `meta/muse-spark-1.3-contributor` ([decision 0021](docs/decisions/0021-default-model.md)).

Useful flags:
- `-C dir`, `-model`, `-reasoning` and `-log path`.
- `-json`: headless events as JSON lines.
- `-no-checkpoints`: turn Jev off.
- `-jev zen`: call [OpenCode Zen](https://opencode.ai/docs/zen)'s free Jev with `ZEN_API_KEY`. It matches the pinned model within its own noise ([decision 0017](docs/decisions/0017-opencode-zen.md)).

Every shell command passes a tripwire first. ast-grep parses the command, and code blocks what can never be allowed: recursive deletes outside the project, force-pushes to a shared branch, and secrets sent off the machine. Commands that delete, push and send nothing run at once. Jev judges the rest. A blocked command hands the run back to you. `-tripwire=false` turns it off. See [decision 0014](docs/decisions/0014-ast-grep-with-jev.md).

Every session writes an event log, including each Jev decision, to `~/.local/state/girdle/sessions/`.

## The fast flow

`-fast` turns on the fast flow, built to take as few LLM steps as it can:

- **The repository goes out with the request.** For a large repository, the request carries the code it names. Jev also picks the other files it will need, one isolated question per file, skipping test data.
- **Changes arrive in one step.** One `apply` call makes every change and runs a check.
- **Searches come with context.** A search result names the definition each match sits in, and shows it when there are few.
- **Jev ends the run as soon as the check's result shows the task done.** It reads the changes and the check's output. When a request renames or removes something, Girdle doesn't stop while the old name is still there.
- **An independent test runs alongside.** A second call writes a test from the task's words, and Girdle runs it once the agent's own check passes.

Any part can be left out by setting its flag to false, for example `-fast -crosscheck=false`.

One part is bought speed, not design: each LLM call is raced three times, and the first answer wins.

Results so far, all side by side with the same model:

- **Muse Spark, small and scale suites.**
  - Warm (a reused, warmed directory, as in daily use): twice as fast as the default flow, at the same pass rate.
  - Cold, on real repositories: 1.24 times as fast.
  - See [research/06](research/06-optimisation-stage-summary.md) and decisions [0005](docs/decisions/0005-fast-flow.md) to [0013](docs/decisions/0013-jev-fan-out.md).
- **Space Bunny, the hard suite's development tasks, warm.**
  - 83 of 99 runs passed. The median passing run took 64 s, at about $0.003 a run: only Jev is paid.
  - Grep context cut steps by 19% and time to 0.87 there. On the held-out tasks it was neutral.
  - Not yet measured cold on this model.
  - See decisions [0019](docs/decisions/0019-harness-time.md) and [0020](docs/decisions/0020-round-trips.md).
- **Where the time goes on Space Bunny.** LLM steps are 86% of a passing run, and each step costs about 2 s before it writes anything. So fewer steps is the lever that works. Showing the model more code up front made no difference.

## Benchmark

`bench/` runs agents on tasks with hidden tests and scores them the way SWE-bench does:

```bash
bench/validate.sh                 # every task fails untouched and passes its reference fix
bench/gate.sh                     # Segment 1 gate scenarios

# A new idea: the fast flow against the fast flow with the idea, warm, on the tasks it targets.
# Any Girdle flag goes after a +, so girdle-fast+crosscheck=false is an ablation.
bench/bench.sh -w -s hard -a "girdle-fast girdle-fast+grepctx=false" -r 3
python3 bench/compare.py bench/results/<run> girdle-fast girdle-fast+grepctx=false

# Two builds side by side: girdle-fast@base runs bin/girdle-base.
bench/bench.sh -w -s holdout -a "girdle-fast@base girdle-fast" -r 2
```

The suites:

| Suite | Tasks |
|---|---|
| `tasks` | Small fixtures |
| `scale` (`-s scale`) | Six tasks on real repositories (goldmark, more-itertools) |
| `hard` (`-s hard`) | 11 development tasks rebuilt from real fix commits, checked by the commits' own hidden tests ([decision 0015](docs/decisions/0015-hard-suite.md)) |
| `holdout` (`-s holdout`) | 22 tasks that are never used for tuning, run only at milestones ([decision 0016](docs/decisions/0016-no-benchmark-fitting.md)) |

Upstream repositories are named in a task's `source` file and cloned once into `bench/.cache`, so their code is never vendored here.

`bench/compare.py` gives the verdict:

- time over passing runs only, since a false "done" is fast;
- a 95% interval across tasks;
- pass counts;
- tool calls per passing run;
- fresh tokens, so that speed bought with compute is named as such.

Two identical builds have measured 16% apart on time, so keep nothing on a single round.

The benchmark's defaults:

- **Model.** Space Bunny, free and public code only. Override with `BENCH_MODEL`, `BENCH_PROVIDER` and `BENCH_JEV`.
- **Isolation.** Girdle's shell commands run offline (`-offline-tools`). They can't read other copies of the code under test anywhere on disk (`-deny-read`): the benchmark's clones, the Go module cache, other projects' `vendor/` and `site-packages`. Agents do go looking for the upstream fix ([decisions 0018](docs/decisions/0018-space-bunny-offline-tools.md) and [0019](docs/decisions/0019-harness-time.md)).

Early and private. See [CLAUDE.md](CLAUDE.md) for the North Star and how we work, and [research/](research/) for the background.

Licensed under Apache-2.0.
