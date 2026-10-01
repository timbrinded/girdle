# 0002 Jev client and checkpoint structure

**Date:** 2026-09-26 · **Segment:** 1

**Decision:**

- `internal/jev` is a ~150-line HTTP client for `POST /v1/systemone`. It retries on 429, 529 and 5xx, and is pinned to `jev-1.13.0`. (`jev-1.13` is rejected as an unknown model.)
- `internal/checkpoint` defines each checkpoint as a question map plus a `Policy` of thresholds. `Decide` maps answers to `stop`, `nudge` or `ask`, and returns the rule that fired. All of a checkpoint's questions go in one request.
- The first checkpoint is `turn_end`. It asks a status Choice (done / in_progress / needs_user / stuck), an evidence Noul, and a needless-permission Noul.
- If Jev errors, the run hands control to the user (`jev_unavailable`) rather than guessing.

**Why:** It keeps "checkpoints are data" without a config format we don't need yet. Go values are enough, and a new behaviour is a question plus a rule. Pinning the version keeps thresholds valid.

## Update 2026-09-26: requirement coverage and routing

- **Requirement coverage.** `SplitRequirements` (code) breaks the task into list items plus lead-in sentences, or otherwise sentences of at least three words, capped at 10. The turn-end request adds two Nouls per requirement: "has this been done?" and "is this an instruction rather than a description?". When status is `done` with evidence, an instruction scoring under 0.5 on "done" triggers one `coverage` nudge that names it. Descriptions ("`go test -race` fails.") are skipped. Without that check they caused a false nudge in the benchmark.
- **Routing.** A `route` checkpoint scores the request's complexity (Score, 0–2) once per run. Code maps it to a reasoning effort for that run's LLM calls: low below 0.6, high from 1.6, medium otherwise. `-no-route` pins the effort to `-reasoning`.
- Everything is still one Jev request per checkpoint.
