# 0012 Prune what didn't pay, and test each new idea on its own

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

After 0011 every idea of the optimisation stage had been measured. Declined ideas still left code behind, and each benchmark round re-ran both suites against the default flow. This record clears the code and changes how ideas are tested.

## Removed

- **The fast second model.** This removes the `-fast-model`, `-fast-provider`, `-fast-all` and `-fast-effort` flags, and the model picking, fallback and per-model token accounting behind them. It also removes the benchmark's `-mix`, `-oss`, `-osshigh` and `-mixhigh` variants.
  - **Why:** it lost on accuracy and cost (0010), and it ran through every LLM call's code path.
  - **To bring it back:** the code is in git history at `0216d29`. Try again only when a candidate model passes the scale suite on its own.
- **`-lean`, and the benchmark's `-r<N>` and effort suffixes.** A flag set explicitly now overrides what `-fast` turns on, so any ablation is written as the fast flow plus a flag. For example, `-fast -reproduce=false` leaves reproduce out. In the benchmark the same run is the agent `girdle-fast+reproduce=false`.

## Shelved

- **Compaction** is off in `-fast`. Its code, tests and the `-compact` flag stay.
  - **Why:** it ran 4 times in 521 runs and never pruned anything. These suites have no sessions long enough to need it.
  - **When to revisit:** once a long-task suite exists.

## Kept, although the plan was to shelve it

- **The heartbeat stays in `-fast`.** The plan was to shelve it with compaction, but the logs show it acts when a model struggles.

  | Runs with the heartbeat on | Runs | Heartbeat checks | Runs nudged | Nudged runs that passed |
  |---|---|---|---|---|
  | Muse Spark | 372 | 80 | 4 | 4 |
  | gpt-oss-120b as the fast model | 149 | 96 | 40 | 30 |

  - It catches a struggling model, which is item 2 of the North Star.
  - It costs about 0.5 s every 6 steps, and most runs end sooner.

## Kept

- **Cross-check, reproduce, API-name hints, the step-end guard, and logging of reasoning summaries.** Each either earned its place or costs nothing until it triggers.

## How ideas are tested from now on

- **A new idea runs against the current fast flow only,** as `girdle-fast` against `girdle-fast+<flag>`, warm, on the tasks it's meant to help.
- **Declined and shelved ideas aren't re-run.** Their decision records hold the evidence.
- **The default flow and both full suites run only at milestones,** such as before a write-up.
