# 0022 One place for feature dependencies, Jev calls and turn halts

**Date:** 2026-09-28 · **Branch:** `feat/scale-bench`

A strict code-quality review of the branch restructured the code without changing what Girdle does. The following choices were not obvious from the code alone:

- **Feature dependencies live in `kernel.Config.resolved`.**
  - The rules were written twice before: once as `&&` chains in `main.go`, and again as checks scattered through the kernel (`routingOn`, `earlyStopOn`, `s.cfg.Jev != nil`).
  - Now `NewSession` resolves the config once. For example, cross-check needs batch and early stop, and early stop needs checkpoints and Jev.
  - After that, the kernel reads one field per feature. `main.go` only maps flags to features.
  - The session-start log records the resolved values, as it did before.
- **Every checkpoint makes its Jev request through `checkpoint.ask` and records a `Call`.**
  - This removed eight copies of the same timing, error and token handling.
  - A missing client and a failed request now fall back the same way everywhere.
  - So does a reply that leaves a question unanswered. Read as zero, a missing risk answer let the tripwire allow a command Jev never judged; an unanswered question is never consent.
  - `Call` is embedded, so its JSON fields stay at the top level of each decision in the log, under the same names. A test pins this.
  - The compact and cross-check events now carry their decisions, `jev_model` included. Before, both were left out of the log.
- **The kernel stops a turn with one typed `halt`.**
  - It replaces the `tripped`, `pending` and `pendingWhy` strings, and the magic value `"blocked"`.
  - When two halts arrive in the same step, the more serious one wins, so a blocked command still beats a failed cross-check.
  - Halts are reset with the rest of a request's facts. Before, a pending nudge could leak into the next request after an error.
- **Text clipping lives in `internal/clip`.**
  - It replaces nine local helpers, line clipping included. Every result is valid UTF-8, including the headless printer and Jev error bodies, which could cut through a character before.
  - The visible changes are small:
    - A line cut in search results now ends `…` rather than ` …`.
    - A cut hint list now ends `… N more lines` rather than `… and N more`.
- **Reproduce claims a reproduction only on evidence.**
  - Before, any failure with the fix undone was reported as "the test reproduces the bug". That included a command that wasn't found, a timeout, and pytest collecting no tests. Early stop could then take that claim as proof.
  - Now the exit-code facts code can read (refused, 126, 127, cut short) are reported as "could not be run".
  - Any other failure is sent to a new Jev question, `test_ran`: did the test itself run and fail, or did it never run?
  - Only a yes is reported as a reproduction. With no answer from Jev, nothing is claimed.
  - The check's result stays as it was either way: only the claim is withheld.
- **Every leftovers call to Jev is logged.**
  - The intent and must-change calls now go to the event log with their answers.
  - Before, must-change's answers were dropped, and a failed request left no trace.

Also:

- apply now builds its result from the shell result's fields. Before, it cut the exit-code line out of the output text and put it back.
- A refused cross-check is logged as "not run". It is no longer sent to Jev as a failing test.
- `FindDefinitions` takes the file list its caller already has. The snapshot no longer runs `git ls-files` once for each name.
