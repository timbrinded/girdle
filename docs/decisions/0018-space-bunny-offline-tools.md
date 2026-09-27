# 0018 Space Bunny Alpha for benchmarks, offline tool commands, and a tripwire fix

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

## A free model for benchmarks

- **Decision.** The benchmark's default model is now `stealth/space-bunny-alpha` on OpenRouter, which is free for now. Girdle's own default stays Muse Spark 1.3 Contributor.
- **Why.** Benchmark rounds are where the money goes. Of the free models on OpenRouter that make proper tool calls, Space Bunny Alpha answered fastest among the capable ones, and the user chose it.
- **Caveats.**
  - It is an anonymous "stealth" model, so it may change or disappear without notice.
  - Its provider may log prompts, so it is for public repositories only.
  - Results on it can't be compared with the Muse results before it. Each change is compared side by side on the same model.
  - Jev's thresholds were tuned on Muse transcripts.
- **Cost accounting.** `score_run.py` now prices each run by the model it used (`SCORE_MODEL`). Free models cost nothing, and only the pinned Jev is paid.

## Development half, first results

These are the fast flow's valid runs on the 11 development tasks, 2 each. gm-470's first two Space Bunny runs are replaced by two runs made with offline tools, for the reasons below.

| | Space Bunny Alpha | Muse Spark, same suite, earlier the same day |
|---|---|---|
| Passed | 19 of 22 (86%) | 18 of 22 (82%) |
| Mean time | 229 s | 284 s |
| Median time | 101 s | 128 s |
| Cost per run | $0.006 (Jev only) | $0.037 |

- **Not side by side.** Hours apart, and one task of difference is within noise. The cost difference is the solid result.
- **Space Bunny solved tm-toml11 in both runs, with no network use.** Muse never solved it in five.
- **Offline, gm-470 failed both runs as false early stops.** Its only pass had come from cloning the upstream fix.
- **A new failure mode.** Space Bunny's ex-pipe-operator change made the parser loop, and it then re-ran hanging tests with long timeouts (400 s, 300 s, 300 s) until the run's limit. A check that runs far longer than the same check's usual time is a strong sign of a hang. It is worth a checkpoint: Jev can judge whether the change caused it.
- **Zen's free Jev ran out of quota in this round's first attempt.** Every request got 429 with `Retry-After: 35824`, and runs hung in the Jev client, which obeyed it. That round was discarded. The client now gives up on waits over 8 s, and the benchmark uses the pinned Jev (decision 0017).

## Offline tool commands for benchmarks

- **What happened.** On gm-470, Space Bunny ran `git clone` on upstream goldmark, whose main branch contains the fix the task is built from, and passed. In the other run it downloaded a released goldmark version with the fix. In about 600 earlier runs, Muse never fetched anything.
- **Decision.** `-offline-tools` runs every shell command through macOS's `sandbox-exec` with outbound network denied, except to this machine. Tests that start local HTTP servers still work, and dependencies are already cached by the warm-up. The benchmark passes it to every Girdle agent. A run that fetched code before this, like gm-470's first Space Bunny run, doesn't count.
- **Elsewhere.** On other systems, offline mode refuses to run commands rather than run them with network.

## The tripwire's first false block

- **What happened.** Space Bunny's command deleted `/tmp/gmdl`, a scratch directory, and also downloaded a file, so it went to Jev. The code had parsed the delete as a temp-directory path, which the floor allows. But Jev's question asked about deleting "outside `project_dir`", and `/tmp` is outside it, so Jev answered 0.92 and the command was blocked.
- **Fix.**
  - The question now excludes temp directories, which it is shown.
  - When code has resolved every deleted path and found none outside the project or the temp directories, Jev's answer on deleting outside no longer blocks.
  - Facts beat a judgement where code has them.
