# Optimisation stage summary: where the fast flow stands

26 to 27 September 2026. Branch `feat/scale-bench`. Decisions 0005 to 0013.

## Headline

In warm, daily-use conditions, the fast flow (`-fast`) is twice as fast as the default flow on both benchmark suites, at the same pass rate. It runs on the same model, Muse Spark 1.3 Contributor, at about the same cost per run.

| Suite, warm | Fast flow | Default flow |
|---|---|---|
| Real repositories (goldmark, more-itertools), 6 tasks | 18/18, 33.2 s mean | 18/18, 66.2 s mean |
| Small fixtures, 13 tasks | 39/39, 19.4 s mean | 39/39, 38.8 s mean |

Cold, with a fresh copy every run, the gap is 1.24× on real repositories and 2.0× on the fixtures (0011).

## What the fast flow does

1. **Snapshot.** The request carries the repository. Small repositories go whole. Large ones get the file list, their agent instructions, and the definitions, uses and neighbouring tests of the code the request names. Jev then judges every other small file, one isolated question each, and the likeliest go too (0013).
2. **One lookup.** Fetches every file, definition and search the model needs in one call.
3. **One apply.** Makes every change and runs a check that proves the task. A bug fix carries a regression test, and `reproduce` shows that test fails without the fix.
4. **Early stop.** Jev reads the changes and the check's output with 59 questions in one request. It ends the run when the work reads as done and verified by a check that exercises the task, with a guard that tests the task asked for have been written (0013).
5. **Cross-check.** An independent test, written from the task's words in parallel, runs before Jev is asked. Jev screens out cross-checks that fail through their own fault.
6. **Racing and speculation.**
   - The first call races three copies.
   - Later calls are hedged after 3 s.
   - The first call starts before routing has finished.
7. **Safety nets.**
   - API names attached to failed checks.
   - A stuck-and-drift heartbeat.
   - Whitespace-tolerant edits.
   - Pipefail checks.

Compaction of stale output is shelved: it is built, but off in `-fast` until a suite has sessions long enough to need it (0012).

## Explored and declined, with the evidence

| Idea | Why not |
|---|---|
| Split the implementation and tests into parallel calls | The implementation half does most of the reasoning, so no faster |
| Race 5 copies | The first of five to finish is often the one that reasoned least, so more checks failed |
| Minimal or no reasoning effort | Minimal lost spec-heavy tasks; "none" is rejected by this model |
| A fast second model (gpt-oss-120b) | 3 to 6 times faster per call, but 11/18 to 13/18 on real repositories, and 3.5 to 5 times the cost. Code removed in 0012 |
| Reuse losing race copies after a failed check | At most 7.9% of run time, realistically far less, and complex |
| FFF (indexed search) | Search is under 0.1% of run time at these sizes; revisit for monorepos |
| Warm the build cache, reorder the prompt for caching | Mainly benchmark artefacts; warm mode measures real use instead |
| Reasoning summaries into Jev | No change in 20 replayed decisions; the misses are blind spots |
| One Jev question per spec rule | Worse than chance at catching false "done"s: Jev can't run code in its head (0013) |
| Jev judging whether tests or code are at fault | Accurate, at 0.97 AUC, but the agent already fixes the right side (0013) |

## What would move it next

- **Bigger and harder benchmarks.** Long, exploration-heavy tasks are where compaction, the heartbeat and API-name hints are built to help, and this benchmark barely exercises them. A SWE-bench-style subset is the next step.
- **A better fast model.** It needs a model that holds up on multi-step work on its own. The mixed-model code was removed in 0012 and can be restored from git history.
- **The long pole: learning an unfamiliar API.** gm-strike-tag stays roughly level with the default flow. That's a knowledge gap in the model, not the flow.

## How new ideas are tested

Each new idea runs against the current fast flow only, warm, on the tasks it targets: `girdle-fast` against `girdle-fast+<flag>`. The default flow and both full suites run only at milestones (0012).
