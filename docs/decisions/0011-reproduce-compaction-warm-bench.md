# 0011 Regression-test proof, compaction, warm benchmarks, and reasoning for Jev

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

## Decisions

- **Reproduce** (on with `-fast`).
  - **What it does.** apply takes an optional `reproduce` command for a bug fix. After a passing check it undoes the request's code changes for a moment and runs the command, then restores the changes. The tools remember each file's content from before the request first changed it, so fixes made across several applies are undone together.
  - **Why.** A test that passes on the old code doesn't reproduce the bug, so that apply's result becomes a failure and isn't taken as proof.
  - **Measured.** In the final round the agent set it 36 times. It confirmed 20 regression tests and caught 4 that didn't reproduce the bug, which the agent then fixed.
- **Compaction** (on with `-fast`; shelved and off in 0012).
  - **What it does.** Every 8 steps Jev judges which older tool outputs over 2 KB are still needed. The 4 most recent are always kept. The rest become a one-line stub in every later prompt. Stubs are only ever added, so the prompt prefix changes rarely and stays cached.
  - **Measured.** Runs on these suites are short, so it ran 3 times and pruned nothing. It is built for long sessions this benchmark doesn't have.
- **`-lean`** leaves both out, for comparison. Removed in 0012: `-fast -reproduce=false` does the same job now. Neither changed speed measurably:
  - Scale, warm: 33.2 s with both, 40.8 s without.
  - Scale, cold: 50.6 s with both, 44.3 s without.
- **Warm benchmark mode** (`bench.sh -w`). Each agent works each task in one reused directory, warmed once by running the check and reset with git between reps, the way someone works in their own checkout. Its path stays the same, so compiler caches and prompt prefixes behave as they do in daily use. Cold mode, a fresh copy every run, remains the default.
- **Reasoning summaries are logged, not used.** Muse Spark's reasoning is encrypted, and OpenRouter returns only a short summary, which is now logged as a `reasoning` event.

## Results (3 runs per task, three variants side by side)

| Round | Fast flow | Fast, `-lean` | Default flow | Default ÷ fast |
|---|---|---|---|---|
| Scale, warm | 18/18, 33.2 s | 18/18, 40.8 s | 18/18, 66.2 s | 1.99× |
| Scale, cold | 18/18, 50.6 s | 18/18, 44.3 s | 18/18, 62.9 s | 1.24× |
| Small, warm | 39/39, 19.4 s | 38/39, 20.6 s | 39/39, 38.8 s | 2.00× |
| Small, cold | 38/39, 22.1 s | 39/39, 19.4 s | 39/39, 45.0 s | 2.03× |

**Warm mode matters most on real repositories.** Cold, each run gets a new path, so the fast flow's large first prompt (snapshot and instructions) is never cached, and its fewer, larger steps lose most of their edge. Warm, as in daily use, the fast flow is twice as fast on both suites.

## Reasoning into Jev: explored, no effect

- **Method.** 38 fast runs were collected with the summaries logged. The 20 step-end decisions that had a summary were replayed through Jev three ways: as recorded, re-asked without reasoning, and re-asked with the last two summaries.
- **Result.** The decisions were the same all three ways: 18 or 17 correct stops of 19, and the one wrong stop still made.
- **Why.** The wrong stop was a js-slugify truncation bug. Its summary talked about diacritics, not truncation. The summaries describe what the model intends, and the remaining misses are blind spots the model doesn't know it has.
- **When to revisit.** A model that exposes its full reasoning, or tasks long enough for loops to show, may change this.
