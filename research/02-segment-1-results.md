# Segment 1 results: first working loop, and Girdle vs Pi

26 September 2026. Issue: timbrinded/girdle#1.

## What was built

| Part | Where | Notes |
|---|---|---|
| Jev client | `internal/jev` | ~150 lines. `POST /v1/systemone`, retries on 429/529/5xx, pinned to `jev-1.13.0` |
| Checkpoints | `internal/checkpoint` | **turn_end**: status Choice, evidence Noul, needless-permission Noul, and per-requirement "done?" plus "is this an instruction?" Nouls, all in one request. **route**: complexity Score → reasoning effort |
| Kernel | `internal/kernel` | Fantasy runs the inner LLM ↔ tools loop. The kernel asks Jev at every turn end and then stops, nudges or hands back. Writes a JSONL event log |
| Tools | `internal/tools` | read, write, edit, bash (the same four Pi ships) |
| TUI | `internal/tui` | Bubble Tea v2. Streaming transcript, tool calls, Jev decisions inline, usage line |
| CLI | `cmd/girdle` | TUI by default. `-p` for headless runs (exit 0 done, 2 needs you), `-seed` to start mid-conversation |
| Benchmark | `bench/` | 13 tasks with hidden tests and reference solutions, a Pi runner in tmux, scoring, and the gate |

**Gate (issue #1):** `bench/gate.sh` passes both scenarios on the final code, using real Jev and a real LLM.

- `rename-across-files`: finished unattended, tests green, decisions logged.
- `announce-then-stop`: Jev read the stalled turn as `in_progress` (confidence 1.00, evidence 0.03). It nudged, the LLM did the work, and the next decision was `done`.

## Benchmark

- **Tasks:** 13 tasks across Go, Python and JS: renames, bug fixes, spec implementations, a multi-file API migration, a multi-part config change, a race fix, and dead-code removal.
- **Checks:** each task's `check.sh` runs hidden tests the agent never sees. `bench/validate.sh` proves every check fails on the untouched repo and passes on a reference solution.
- **Model:** both agents use `meta/muse-spark-1.3-contributor` via OpenRouter, headless, with no human input.
- **Pi:** vanilla, in tmux, with its own config dir, so no extensions and retries on.
- **Cost:** computed from token counts at OpenRouter prices ($0.10/M input, $0.002/M cache read, $0.20/M output), plus Jev at $0.042/M.

### Iterations

| Run | Girdle change | Girdle pass | Girdle time | Girdle $/run | Pi pass | Pi time | Pi $/run |
|---|---|---|---|---|---|---|---|
| baseline (8 tasks, 3 reps) | first loop | 22/22 | 62 s | 0.0026 | 21/23 | 32 s | 0.0023 |
| v2 (13 tasks, 3 reps) | fixed optional tool params; requirement coverage; routing (mostly medium) | 38/38 | 78 s | 0.0030 | 36/38 | 46 s | 0.0030 |
| low-effort experiment (6 spec-heavy tasks, 3 reps) | fixed low effort | 18/18 | 74 s | 0.0026 | 17/18 | 49 s | 0.0030 |
| v3 (13 tasks, 3 reps) | route most work to low; UTF-8 fix; instruction filter | 38/39 | 55 s | 0.0023 | 39/39 | 57 s | 0.0037 |
| **v4 final (13 tasks, 5 reps)** | toolchain hint in the prompt | **65/65** | **49 s** | **0.0023** | **65/65** | **53 s** | **0.0031** |
| v5 smoke (13 tasks, 1 rep, Girdle only) | step summaries keep the end of tool output | 13/13 | 55 s | 0.0022 | – | – | – |

The v4 figures come from the code just before the last kernel fix, which stops step summaries clipping away test results. The v5 smoke run and the final gate ran on the fixed code. Girdle passed every task with no nudges, at about the same cost.

In the low-effort experiment, Pi and the Jev-off Girdle also ran at low effort. Excluded as benchmark bugs: two `go-ttl-cache` failures (one Girdle, one Pi in v2) caused by my hidden test's helper names colliding with the agent's own. All `go-rename` results before the fixture was restored were also discarded.

**Result:** with the same model, Girdle matched vanilla Pi's reliability (100% vs 100% over 65 runs each). It was **8% faster and 26% cheaper per run**. Pooled over every valid run (excluding the corrupted baseline `go-rename` runs and the two name-collision runs), Girdle passed 178/179 and Pi 175/180.

### What Jev actually did (124 Girdle runs)

- 120 turn-end decisions, 81 routing decisions.
- Median latency 247 ms per turn-end decision, 251 ms per routing decision.
- About 2k Jev tokens per run: roughly $0.00008, or 3% of a run's cost.
- **Routing was the measurable win.** Low reasoning effort plus the turn-end checks passed every spec-heavy task that medium did, in about half the time.
- **Turn-end checks rarely changed an outcome on these tasks.** 119 of 120 decisions were "done → stop". On small, well-specified tasks Muse Spark finishes cleanly in headless mode.
- The only real nudges were two in v4, on one run. They came from state that was too lossy (step summaries clipped away the passing tests) and have been fixed. One earlier nudge came from treating a description as a requirement, also fixed.
- The "leave it alone" value is proven by the `announce-then-stop` gate, where Pi would simply have exited. The benchmark doesn't exercise it much. That needs longer or interactive tasks.

## Bugs found by running it end to end

1. **Optional tool parameters were sent as required.** Fantasy reads `omitempty`, not `omitzero`, so every bash call first failed on `timeout_seconds`.
2. **Invalid UTF-8.** Clipping at byte offsets split accented characters. Go's `encoding/json/v2` rejected the result, which silently broke the Jev request (→ `jev_unavailable`) and the log write. Clipping now respects character boundaries and sanitises output.
3. **Wrong Jev model ID.** It is `jev-1.13.0`; `jev-1.13` is rejected. The fallback worked correctly: the run handed control to the user.
4. **Benchmark harness bugs** (paths, a bash 3.2 empty-array bug, the tmux watchdog holding pipes, cost double-counting, hidden-test name collisions). One run edited the fixture repo because Pi ran in the wrong directory.

## Caveats

- 13 small tasks, one model, headless only. The benchmark is small enough that one run is ±1.5 percentage points.
- The routing cut points (low below 1.7, high from 1.9) were tuned on this benchmark. Retune them from real decision logs.
- Pi was vanilla. Your own Pi setup, with its extensions, might do better or worse.
- The model is the Contributor tier: Meta trains on prompts. Don't point it at private code.

## Next segments (one line each, to be written after this gate)

- **Segment 2: stuck and drift.** A heartbeat checkpoint that catches repeated failures and drift within a few steps, using longer tasks that exercise it.
- **Segment 3: tripwire.** A never-allow list in code plus a Jev damage Score on tool calls, stopping only catastrophic actions.
- **Segment 4: context.** Relevance pruning of tool output and compaction for long sessions.
- **Segment 5: sessions.** Resume, undo, and a decision-log view for threshold tuning.
