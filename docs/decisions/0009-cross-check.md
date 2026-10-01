# 0009 An independent cross-check of each request

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

**Context:** With 0008 the fast flow was 1.5 to 2.5 times faster than the default flow. It still missed an occasional hidden test that the default flow passed. Each miss was a spec edge case that the model's own tests agreed with:

- `argsort` consumed an iterator twice. The task said "iterable", and the model's tests used lists.
- A line total of 7 × 1.005 was rounded in floats, giving 7.03 instead of 7.04. The model's tests only used exact values.

A one-shot answer gets fewer chances than the default flow's step-by-step work to notice such a gap. The same model tends to test what it thought of when it wrote the code. Telling it to derive edge cases from the task's words helped only partly.

**Decision:** `-crosscheck`, on with `-fast`.

- **A separate writer.** When a request starts, a second LLM call begins alongside the main one. It uses the plain model at low effort, outside speculative routing. Since the update below it is also raced. It sees the task and the snapshot, never the agent's code.
- **What it writes.** One small test file from the task's words: the stated cases, and the edge cases they imply. It uses literal expected values worked out by hand, never computed in the test. The file is new, with `girdle_crosscheck` in its name, and its check runs only that file. Any other change, such as an edit to an existing file, makes the cross-check void.
- **When it runs.** The first time the agent's own check passes, before Jev is asked, Girdle writes the file, runs its check with pipefail, and removes it again, along with any `__pycache__` copies. It runs once per request, and waits at most 15 s for the writer.
- **When it fails.** The agent gets the test and its output, and decides whether the test or its code is wrong. It either fixes the code and adds the case to its own tests, or says in a sentence why the test is wrong. Jev's turn-end checkpoint then judges as usual.

**Why:** Tests written apart from the code don't share its blind spots. That is also how the benchmark's hidden tests catch these misses. The writer runs in parallel, so a passing cross-check usually adds only its own run time. Its LLM call costs about as much as one step.

**Risk:** A wrong cross-check costs the agent a step to dismiss it. In a smoke run, one test computed its expected value with convoluted code. The agent correctly called it wrong, but the detour cost about 15 s. That's why the writer must now use literal expected values and test only what the task clearly specifies.

## Update 2026-09-26: Jev screens failures, and results

Of the first 8 failing cross-checks on the benchmarks, 5 were the test's own fault and 3 caught real edge cases:

- **Own fault:** a wrong package name, a v1 import path, an ignored error return, and an illegal struct comparison.
- **Real catches:** CRLF kept inside quoted CSV fields, and two slug truncation rules.

- **Jev screens a failure before the agent sees it.** A `crosscheck` checkpoint asks Jev whether the failure is the test's own fault. On those 8 cases it scored the false alarms 0.79 to 0.92, and the real catches 0.05 to 0.10, in about 0.25 s. Only failures under 0.5 reach the agent.
- **The writer is raced 3 ways.** When the agent finishes quickly, the writer is on the critical path, and a slow one was once not ready.
- **The writer follows existing conventions.** It uses the package name and import paths of the existing tests.

**Results with screening, 3 runs per task, side by side:**

| Suite | Fast flow | Default flow |
|---|---|---|
| Scale | 18/18, 40.7 s mean, 30 s median | 18/18, 58.5 s mean, 50 s median |
| Small | 39/39, 23.0 s mean, 16 s median | 39/39, 46.4 s mean, 37 s median |

Cross-check outcomes over those 57 fast runs:

| Outcome | Runs |
|---|---|
| Passed | 38 |
| Real failure, sent to the agent | 2 |
| Screened out by Jev | 2 |
| None written | 13 |
