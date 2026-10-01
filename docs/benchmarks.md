# Benchmarks

The benchmark gives each agent a task and a disposable repository, then runs the task's check against the resulting code. A passing hidden check determines the recorded pass, independently of whether the agent reports completion. Some tasks restore original tests before adding hidden ones, so weakening the agent-visible tests does not establish a pass.

The task statements, checks and reference fixes are public inputs. Keep them out of the agent's working context when measuring it. These experiments measure specific tasks under recorded conditions; they do not establish correctness on arbitrary work.

## Requirements

Girdle **live benchmark runs require macOS** with `sandbox-exec`. The runners always enable `-offline-tools` and `-deny-read`; on Linux the restricted shell commands are refused. Ordinary Linux Girdle use and local fixture validation are separate from this limitation.

The scripts also need Go, Git, Bash, Python 3, zsh and the command-line utilities they invoke, including `timeout`, `xargs`, `tar`, `sed`, `awk` and Perl. On macOS, make sure `timeout` is available on PATH, for example through GNU coreutils. Individual task checks need their language runtimes: Go, Python 3 or Node.js. Install ast-grep for Girdle's structural lookup and tripwire.

Live Girdle runs need the selected LLM and Jev keys. The scripts attempt to load missing keys through an interactive zsh; exporting them beforehand makes the configuration explicit. The default agent set includes Pi, which additionally needs a working `pi` binary and tmux. Specify only Girdle agents to avoid that requirement.

Live runs call providers and can incur costs. Fixture validation does not call LLMs or Jev, although preparing upstream tasks can clone repositories and download dependencies.

## Task suites

| Suite | Tasks | Use |
| --- | --- | --- |
| `tasks` | 13 small Go, Python and JavaScript fixtures | Development; default suite |
| `scale` | 6 tasks on goldmark and more-itertools | Development on larger repositories |
| `hard` | 11 tasks reconstructed from real upstream fixes | Development on harder work |
| `holdout` | 22 tasks, including repositories outside development | Milestone evaluation; never tune from these results |

Each task has `task.md`, `check.sh` and a reference fix (`solution/` or `solution.patch`). Small fixtures include `repo/`; upstream tasks instead have a `source` URL and pinned starting commit, and sometimes a `setup.patch`. Hidden tests live under `hidden/`.

[prepare.sh](../bench/prepare.sh) clones upstream repositories into `bench/.cache`, copies the starting revision into a fresh working directory and creates a Git baseline. Full upstream repositories are not bundled; the patches and derived tests have [third-party notices](../THIRD_PARTY_NOTICES.md).

## Validate a task first

From the repository root:

```bash
bench/validate.sh bench/tasks/go-rename/
```

Validation checks that the starting repository fails and the reference fix passes. To validate all suites at a milestone:

```bash
bench/validate.sh
```

The full command includes held-out fixtures. Validating their checks is different from evaluating an agent on them; neither activity licenses tuning from held-out content or results.

## Run a comparison

For a new idea, compare the current fast flow with the changed flow on development tasks it can affect. This small example measures grep context on development tasks:

```bash
bench/bench.sh -w -s hard -t "gm-470 gm-link-237" \
  -a "girdle-fast+grepctx=false girdle-fast" -r 3 -j 2
python3 bench/compare.py bench/results/<run> girdle-fast+grepctx=false girdle-fast
```

Replace `<run>` with the result directory printed by the runner. Without `-w`, each run starts in a fresh directory. Warm mode reuses each agent/task directory, warms its check once, and resets it between repetitions. Provider cache state and latency can still vary in both modes.

| Option | Default | Meaning |
| --- | --- | --- |
| `-a "agents"` | `girdle girdle-nojev pi` | Space-separated agents or variants |
| `-s suite` | `tasks` | Select a task suite |
| `-t "tasks"` | Every task in the selected suite | Select task names; put after `-s` |
| `-r n` | `3` | Repetitions per agent/task |
| `-j n` | `4` | Concurrent jobs |
| `-o dir` | Timestamp under `bench/results` | Output directory |
| `-w` | Off | Reuse warmed directories |

Agents are `girdle`, `girdle-nojev`, `girdle-fast` and `pi`. A Girdle variant accepts flags after `+`, such as `girdle-fast+crosscheck=false`. A build label after `@` selects `bin/girdle-<label>`:

```bash
go build -o bin/girdle-base ./cmd/girdle
# After making the candidate change, run both builds:
bench/bench.sh -w -s hard -a "girdle-fast@base girdle-fast" -r 3
python3 bench/compare.py bench/results/<run> girdle-fast@base girdle-fast
```

The runner rebuilds `bin/girdle`; it retains the labeled build. Pin the intended commits and retain their hashes with your report.

| Environment | Default | Meaning |
| --- | --- | --- |
| `BENCH_PROVIDER` | `openrouter` | Girdle LLM provider |
| `BENCH_MODEL` | `stealth/space-bunny-alpha` on OpenRouter | Explicit model for every run; Zen has its own historical fallback |
| `BENCH_JEV` | `typesafe` | Girdle Jev provider |
| `BENCH_REASONING` | `medium` | Reasoning argument; Girdle can still route effort |
| `BENCH_TIMEOUT` | `600` seconds, or `1200` for hard/holdout | External per-run timeout |

Girdle variants use these settings. Pi is always launched with its OpenRouter provider; it receives the selected model and thinking level. The benchmark explicitly passes model and reasoning arguments, so Girdle's saved TUI choices do not affect a run.

At a milestone, a held-out comparison is:

```bash
bench/bench.sh -w -s holdout -a "girdle-fast@base girdle-fast" -r 2
```

[gate.sh](../bench/gate.sh) is a separate live Segment 1 gate: a rename and a seeded “announce then stop” conversation. It follows Girdle's saved model/effort defaults because it does not override them.

## Isolation and artifacts

Girdle benchmark shell commands are restricted to localhost network access. Denied path patterns cover the harness checkout, reference fixes, other working copies, and matching upstream copies in module caches or vendored packages. The active working directory remains readable. Girdle's direct file tools also enforce denied paths.

Pi runs with its own agent directory and disables extensions, skills, templates, context files, themes and saved sessions. It does **not** receive Girdle's network or read sandbox. Account and provider settings also remain external inputs. Interpret cross-agent comparisons with that difference in mind.

Each run directory contains logs, the diff, changed-file list, check output and `result.json`. Girdle adds `events.jsonl`; Pi adds `pi.jsonl`. These outputs are ignored by Git and can contain code and provider context. Redact shared examples.

## Interpret the comparison

`compare.py` takes **baseline first, candidate second**. It excludes rows marked invalid and reports pass counts separately from time. Only passing runs contribute to the time ratio; a false “done” is not a speed improvement.

For each task passed at least once by both agents, it compares mean log time. The overall candidate/baseline ratio is a geometric mean over tasks. A ratio below `1` favors the candidate. The 95% interval resamples tasks; it does not capture every source of provider or run-to-run variation.

The current verdict rules are:

- `FASTER`: the whole time interval is below `1`, the candidate has at most one fewer pass, and average fresh LLM tokens rise by no more than 10%.
- `FASTER, BY COMPUTE`: the same time/pass criteria, with more than 10% extra fresh tokens.
- `SLOWER`: the whole interval is above `1`.
- `NO CLEAR DIFFERENCE`: other cases.

Fresh tokens are uncached input plus output, including estimated losing race copies. Cache reads are reported separately. Table row mean times include all valid runs; the aggregate time ratio and the reported passing-run median use passing runs only. Compare pass counts and sample sizes before interpreting speed.

Costs are **estimates**, not invoices. `score_run.py` contains the historical Muse Spark prices and treats recorded free-model cases as zero LLM cost. Other paid models use the Muse prices and set `cost_estimated`; this fallback is not an accurate price quote. Jev is estimated from a recorded per-token rate, or zero through Zen. Racing usage includes estimated losing copies and can exceed billed usage. Confirm current prices and provider billing separately.

## Recorded results

The dated [stage summary](../research/06-optimisation-stage-summary.md) covers Muse Spark's early warm/cold comparisons. [Decision 0019](decisions/0019-harness-time.md) records the shift to passing-only timing, leaked upstream copies and measured noise. [Decision 0020](decisions/0020-round-trips.md) records grep context's development gain and neutral held-out result.

Identical builds measured 16% apart in one recorded comparison. Preserve the model, commits, concurrency, repetitions, warm/cold mode, failures and exclusions in a report, and repeat an apparent improvement before retaining it. Historical reports keep the methodology used at the time; later rules do not silently rescore them.

The original run corpus is not bundled. [Research experiments](../research/experiments/README.md) retain the analysis methods and input requirements, rather than a replayable copy of every original measurement.
