# 0010 Speed ideas explored after 0009: what was kept and what wasn't

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

After 0009 the fast flow was 1.4 to 2.0 times faster than the default flow. This record covers every further idea tried, each measured side by side against the default flow, 3 runs per task unless noted.

## Kept, but neutral on the benchmark

These were cheap and safe, but none moved the benchmark numbers (scale 1.27×, small 1.97× in their run, both within noise of before):

- **Step-end threshold down from 0.8 to 0.7, with a tests-written guard.** In the logs every early stop from 0.80 to 0.85 passed its hidden tests (27 of 27). No run that went on from 0.70 to 0.80 changed code afterwards (0 of 28). Jev's coverage answers called "add tests" done before any test existed, so routing now asks whether the request wants tests. A run doesn't stop early while it does and no test file has changed.
- **API names on failed checks.** When the compiler or runtime names something that doesn't exist, the apply result lists what that package, type or module really exports. It never triggered on the benchmark.
- **Fail-fast fix-up checks.** A fix's check runs the quick test of what failed first, then the full proof.
- **Stuck and drift heartbeat.** Jev judges the trajectory every 6 steps. On real traces it called the known rabbit hole "looping" at 0.67 once, and everything else "progressing". In the benchmark it never fired, 17 of 17 "progressing".
- **Cross-check wait cut from 15 s to 5 s.** In 110 runs the writer finished a median 10.5 s before it was needed. The two times it wasn't ready, the wait bought nothing.

## Declined after measurement

- **A fast second model** (`gpt-oss-120b`).
  - **Speed:** on Groq it answered a 9 KB-prompt request in 0.6 to 0.8 s, against 3.3 to 5.2 s for Muse Spark.
  - **Accuracy on real repos:** every fast-model variant lost to Muse Spark.

    | Variant | Passed | Mean time | Cost per run |
    |---|---|---|---|
    | Muse Spark first, fast model after | 11/18 | 28.9 s | $0.023 |
    | Fast model throughout | 13/18 | 25.6 s | $0.033 |
    | Fast model throughout, high effort | 17/18 | 152.9 s | $0.45 |
    | Fast flow on Muse Spark | 18/18 | 41.6 s | $0.007 |

  - **Why it failed:** most failures ended with Jev judging the run stuck, on the multi-step tasks. On the small suite, running the fast model throughout passed 21/28 in 6.4 s.
  - **Price:** Muse Spark Contributor costs about $0.003 to $0.004 a run. The fast model cost more because it writes far more tokens: about 4k a run, and 78k at high effort, at $0.60 per million. Cheaper hosts exist, such as Crusoe at $0.05 and $0.25, which was nearly as fast as Groq. But the failures are the model's, not the host's.
  - **Kept as opt-in flags** (`-fast-model`, `-fast-all`, `-fast-effort`), with a fallback to the main model when a fast call fails, for a better fast model later. Removed in 0012.
- **Reusing losing race copies after a failed check.** In 114 runs the first apply failed 30 times, always with a losing copy available. Its fix-up took a median 6.3 s. Even if every loser had been right, reuse would save at most 2.4 s a run (7.9%). The copies share a model and a prompt, so they tend to fail alike. It would also need undoing applied changes and rewriting the conversation mid-turn.
- **FFF (indexed fuzzy search).** Lookups take 11 ms and about 0.04 s of a 42 s run. It is worth revisiting only for monorepos, where ripgrep and the snapshot's definition search would take seconds.
- **Warming build caches, and a cache-friendly prompt order.** Both mainly recover costs the benchmark creates: a fresh directory every run, so cold caches and a new prompt prefix. In daily use the checkout is warm and its path doesn't change. Test runtime, not compilation, dominates the checks anyway: goldmark's root tests run about 8.5 s of a 10.7 s apply. The benchmark gained a warm mode (`bench.sh -w`) instead, so it measures what users see.

## Tuning read from the logs

- **Hedging at 3 s is about right for Muse Spark.** 62% of calls after the first ran past 3 s. A hedged copy won 99 of 394 races.
- **The first call's racing works as intended.** Its wins split evenly across the three copies (85, 80 and 81).
