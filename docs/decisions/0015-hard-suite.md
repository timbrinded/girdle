# 0015 A hard suite of real fixes, SWE-bench style

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

## Why

By decision 0014 the fast flow passed every task in both suites, 39 of 39 small and 18 of 18 on real repositories. Speed also moved by up to 50% between identical rounds. So accuracy ideas could only be tested by replaying old failures. Speed ideas needed large effects to show. And the North Star behaviours that need long work were never exercised: noticing when stuck, pruning context, and not stopping to announce or ask.

## What

`bench/hard` holds tasks rebuilt from real fix commits in seven repositories without heavy dependencies:

- expr, an expression compiler and VM
- goldmark
- BurntSushi/toml
- pflag
- go-cmp
- gorilla/mux
- more-itertools

Each task:

- starts from the fix commit's parent, pinned in `source` and cloned into `bench/.cache` as the scale suite does;
- has a task statement written from the commit, describing the behaviour the fix brings, with an example, and naming only the API the hidden tests need;
- keeps the commit's own test and test-data changes hidden, in `hidden/tests.patch`, with the rest of the commit as `solution.patch`;
- is checked the way SWE-bench checks: `check.sh` resets every test file the agent touched, deletes new test files, applies the hidden tests, and runs the affected packages. Only the upstream tests decide, so an agent's own tests can't collide with the hidden ones by name.

## How the tasks were chosen

1. **Mine commits.** Non-merge commits that change both source and tests, with 10 to 250 changed source lines, leaving out bumps, docs, formatting and refactors.
2. **Pick candidates.** Real bug fixes and features, preferring changes across several files.
3. **Validate.** `bench/validate.sh` requires each check to fail at the start and pass with the reference fix. 26 candidates were built, and 4 dropped:
   - one commit's tests already passed at the start;
   - one upstream commit contained merge-conflict markers;
   - one commit's tests also required an unrelated refactor from the same commit, which no task statement could fairly ask for;
   - one commit mixed a test-data move with a feature its tests didn't exercise.
4. **List the API.** Compiling the hidden tests against the starting code lists every name the tests need, and each statement names them.
5. **Calibrate** against the current fast flow; results below.

## Calibration

The current fast flow ran every task two or three times, warm, with a 20-minute limit. The limit was 10 minutes at first, but a correct run took 11 minutes, so hard tasks now get 20 by default.

| Result | Tasks |
|---|---|
| Passed every run | 19 of 22 |
| gm-cjk-runes | 1 of 2: one false "done" |
| tm-toml11 | 0 of 2: one false "done", one out of time |
| gm-470 | 0 of 2: out of time both times |
| All valid runs | 41 of 46 (89%), mean 308 s, from 28 s to the 20-minute limit |

Times are what the suite adds most. The scale suite averages 30 to 40 s, and this one averages five minutes. The single run with most tool calls took 60, and high-effort LLM steps on the biggest tasks took 50 to 150 s each.

### What the failures show

Every false "done" came from a check reading the work as finished when the hidden tests disagreed:
- **The turn-end check.** On gm-cjk-runes and gm-470, the step-end check correctly refused to stop, reading the work as not verified. The model then ended its turn, and the turn-end check accepted "done". That check reads only the final message and a summary of recent steps, not the changes and the check's output.
- **The step-end check.** On tm-toml11 it stopped a run whose 1.1 support was still incomplete.

### Statement gaps found in calibration

14 runs were excluded, and their tasks re-run:
- Four statements missed something the hidden tests required: `slots=True` on a dataclass, the exact wording of an existing error, a precise specification of map ownership, and the expected text for each node type. In each case the agent had built what the statement asked.
- One commit's hidden tests also checked an unrelated refactor from the same commit. Its disassembler test was dropped from the hidden tests.
- Runs cut off by the old 10-minute limit were re-run under the new limit.
- Two runs lost their results when a benchmark script was edited while it ran.

Compiling the hidden tests against the starting code finds the missing names, but not requirements like these. A failure in this suite is checked against its statement before it counts.

## Use

```bash
bench/validate.sh bench/hard/*/
bench/bench.sh -w -s hard -a "girdle-fast girdle-fast+<flag>" -r 3
```

Tests run with `go test -vet=off`, since some upstream commits carry unrelated vet failures, and goldmark's timing tests are skipped as in the scale suite. Hard tasks get a 20-minute limit unless `BENCH_TIMEOUT` sets another.
