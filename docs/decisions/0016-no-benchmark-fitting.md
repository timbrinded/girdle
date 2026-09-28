# 0016 Don't fit the benchmark: a held-out set, a cleaned prompt, honest reporting

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

The goal is a harness that is faster and smarter by design, not one tuned to its own benchmark. An audit found fitting in several places:

- **The prompt quoted a hidden test's answer.** After mi-argsort's hidden test caught `argsort` consuming an iterator twice, the fast flow's prompt gained "a one-pass iterator wherever it says iterable, equal items" as general advice (commit d9ff866).
- **Thresholds were tuned on the tasks they were judged on.** That covers the step-end thresholds, prefetch's cut-off and whole-files rule, and the test-helper ranking behind `-structure`. Leave-one-task-out helped, but the pool was the same 19 tasks.
- **Some checks were validated on the failures that inspired them.** The leftover check was built from six rename failures and shown to catch them by replaying those same six.
- **The tasks share one author's style.** 40 of the 41 statements put code names in backticks, which the snapshot's named-code lookup relies on.
- **The headline number was the flattering one.** "2× faster" was the warm result. Cold, on real repositories, it was 1.24×.
- **Some speed is bought, not designed.** The first call races three copies.
- **One author on both sides.** The same author wrote the harness and the tasks.

## Decisions

1. **The prompt no longer carries benchmark knowledge.** The edge-case sentence keeps its general instruction, "work out the edge cases the task's words imply, and make the code handle them and the tests cover them", and loses the examples taken from that hidden test. The effect was measured on the development half only; results below.
2. **A held-out set.** `bench/holdout` isn't run during development, and nothing is designed or tuned from its results. It has two groups:
   - **Half of the hard suite, 11 tasks,** split to balance repositories and difficulty. The fast flow was calibrated on them once, before the split, and no feature was tuned on them.
   - **11 fresh tasks from six repositories** Girdle's development never touched: semver, go-version, go-humanize, gjson, go-toml and gobwas/glob. Several fixes are from this month, so they are unlikely to be in the model's training data. An agent with no knowledge of Girdle wrote their statements from each commit's message and hidden tests, in a user's style. A second agent checked each statement against its tests: 10 had no gaps, and one contradiction was fixed. Girdle has never run on them.
3. **Hidden tests that pin internals are removed.** A test of an unexported helper forces the upstream solution's design on the solver, so gjson's `revSquash` test was dropped, as ex-let-variables' disassembler test was. Where an internal is woven through a whole test, the statement names it: semver's `check` returning `(bool, error)`.
4. **Development uses `bench/tasks`, `bench/scale` and the 11 remaining `bench/hard` tasks.** Thresholds and question wording are tuned there only.
5. **Reporting rules, now in CLAUDE.md.** Cold and warm, cost beside time, the spread across runs. Speed gained by design, meaning fewer steps, is kept apart from speed bought with compute, such as racing.

## The leaked hint, measured on the development half

The fast flow ran with the cleaned sentence against a build with the old examples restored, side by side, warm, 2 runs on each of the 11 development tasks:

| | Cleaned prompt | With the leaked examples |
|---|---|---|
| Passed | 18 of 22 (82%) | 16 of 22 (73%) |
| Mean time | 284 s | 308 s |
| Cost per run | $0.037 | $0.044 |

Removing the examples cost nothing. The difference is within the noise of 2 runs per task, so the claim is only that the prompt didn't depend on them. The temporary flag used to measure this was removed. gm-470 and tm-toml11 failed in both arms, as in calibration.
