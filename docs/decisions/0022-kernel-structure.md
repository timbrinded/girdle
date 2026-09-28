# 0022 One place for feature dependencies, Jev calls and turn halts

**Date:** 2026-09-28 · **Branch:** `feat/scale-bench`

A strict code-quality review of the branch restructured the code without changing what Girdle does. Four choices were not obvious from the code alone:

- **Feature dependencies live in `kernel.Config.resolved`.**
  - The rules were written twice before: once as `&&` chains in `main.go`, and again as checks scattered through the kernel (`routingOn`, `earlyStopOn`, `s.cfg.Jev != nil`).
  - Now `NewSession` resolves the config once. For example, cross-check needs batch and early stop, and early stop needs checkpoints and Jev.
  - After that, the kernel reads one field per feature. `main.go` only maps flags to features.
  - The session-start log records the resolved values, as it did before.
- **Every checkpoint makes its Jev request through `checkpoint.ask` and records a `Call`.**
  - This removed eight copies of the same timing, error and token handling.
  - A missing client and a failed request now fall back the same way everywhere.
  - `Call` is embedded, so its JSON fields stay at the top level of each decision in the log, under the same names. A test pins this.
  - The compaction decision now logs `jev_model` too.
- **The kernel stops a turn with one typed `halt`.**
  - It replaces the `tripped`, `pending` and `pendingWhy` strings, and the magic value `"blocked"`.
  - When two halts arrive in the same step, the more serious one wins, so a blocked command still beats a failed cross-check.
  - Halts are reset with the rest of a request's facts. Before, a pending nudge could leak into the next request after an error.
- **Text clipping lives in `internal/clip`.**
  - It replaces seven local helpers. Every result is valid UTF-8, including the headless printer and Jev error bodies, which could cut through a character before.
  - The one visible change is a line cut in search results: it now ends `…` rather than ` …`.

Also: apply now builds its result from the shell result's fields. Before, it cut the exit-code line out of the output text and put it back. A reproduce command that the tripwire refused is now reported as "could not be run". Before, it was reported as a reproduced bug.
