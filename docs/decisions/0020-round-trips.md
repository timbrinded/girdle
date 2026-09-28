# 0020 Fewer round trips: grep results that show their definitions

**Date:** 2026-09-28 · **Branch:** `feat/scale-bench`

Decision 0019 found that no context or prompt change moved the fast flow's time on Space Bunny: the model thinks about the same amount per task however the code reaches it. One lever was left.

## The lever: round trips

Over 46 passing fast-flow runs:

- **A step costs about 2.0 s fixed, plus 7.8 ms per output token.** The fixed part is 34% of LLM time, about 32 s a run.
- **Space Bunny makes one tool call per step.** Only 3 of 758 steps made more, so every tool use is a whole round trip.
- **So a step that doesn't happen saves its 2 s, and the thinking that comes with it,** even though a bigger or smaller prompt changes nothing.

## Follow-ups: apply takes the likely next step (cut)

- **What it did.**
  - After a passing check, `apply` ran the project's whole test suite. Code found the command from the repository's files: `go.mod`, pytest configuration, `package.json`, a Makefile `test:` target. Jev chose when there were several.
  - If the suite failed, it ran again with the request's changes undone, to say whether the failure was the agent's.
  - `apply` also showed each edited region as it now reads.
- **Why it seemed worth it.** The agent ran the whole suite in a separate step about once a run. About 0.9 times a run it read a file it had just edited.
- **Result.** Round 4, 3 runs per development task, side by side:

  | | Fast flow | Follow-ups |
  |---|---|---|
  | Time ratio (passing runs) | | 0.96 (0.78 to 1.20) |
  | Passed | 27 of 33 | 26 of 33 |
  | Steps per passing run | 17.9 | 17.2 |

  - The suite run fired 0.09 times a run: the agents' own checks already ran `go test ./...`, as the fast-flow prompt asks.
  - Seeing the edited code didn't stop the reads.
- **Cut**, with its command detection.

## Grep context (kept, part of `-fast`)

- **The idea came from FFF,** an MIT-licensed file search engine for agents (github.com/dmtrKovalenko/fff). Its results:
  - mark definition lines;
  - retry fuzzily when nothing matches;
  - rank by frecency and git status;
  - aim for fewer search round trips.
- **MCP is a Girdle non-goal,** so the ideas are built into Girdle's tools rather than wired to `fff-mcp`.
- **What the logs supported.**
  - The agent searches mostly with bash grep, 4.8 times a run, and 1.5 times with `lookup`.
  - Searches that find nothing are rare, about 0.25 a run.
  - A grep was followed straight away by a read of a file it matched 0.7 times a run.
  - So the part worth building is the definition context. Ranking wasn't built, since bash output can't be reordered.
- **What it does.**
  - A search result, from a bash `grep`/`rg` command or a `lookup` search, gets a note naming the definition each match sits in, with its line range. The definition's bounds come from the code: a top-level definition in brace languages, and in Python the innermost function or class.
  - When the matches fall in three definitions or fewer, totalling 150 lines or less, the note shows their source.
  - A `lookup` search that finds nothing is retried ignoring case.
  - Paths that `-deny-read` blocks are skipped.
- **Result.** Three rounds, each 3 runs per development task per arm, side by side:

  | Round | Time ratio (95%) | Passed (grep context vs fast flow) | Tool calls per passing run |
  |---|---|---|---|
  | 4 | 0.89 (0.76 to 1.03) | 29 vs 27 of 33 | 12.5 vs 17.5 |
  | 5 | 0.91 (0.71 to 1.15) | 25 vs 26 of 33 | 14.8 vs 18.2 |
  | 6 | 0.90 (0.76 to 1.06) | 29 vs 29 of 33 | 13.6 vs 15.6 |
  | Pooled, 99 runs a side | 0.87 (0.78 to 0.98) | 83 vs 82 | 13.7 vs 17.1 |

  Pooled, over passing runs and winning copies only:

  | | Fast flow | Grep context |
  |---|---|---|
  | Steps | 17.3 | 14.0 |
  | Output tokens | 8,131 | 7,024 |
  | Mean time | 112 s | 99 s |
  | Median time | 75 s | 64 s |

  - Fresh tokens rose 3% and cost per run didn't move, so this is speed by design, not bought.
  - The verdict rule reads FASTER on the pool, and each round alone pointed the same way.
- **Why it works where `-wide` didn't.**
  - `-wide` put code in front of the model up front. It then took fewer steps and thought longer in each.
  - Grep context shows the definitions the agent asked about, at the moment it asks. That removes the read step and the re-planning that came with it.
- **Held-out confirmation: not confirmed.** The new build ran against the build before it, side by side, 2 runs per held-out task:

  | Held-out | Before | With grep context |
  |---|---|---|
  | Time ratio (passing runs) | | 1.01 (0.87 to 1.20) |
  | Passed | 39 of 44 | 39 of 44 |
  | Tool calls per passing run | 12.4 | 12.3 |
  | Median passing run | 53 s | 63 s |
  | Cost per run | $0.0022 | $0.0022 |

  - Grep context fired 2.1 times a run there, against 2.9 on development.
  - But the held-out agents grepped far less, 3.2 bash greps a run against 8.1, and round trips didn't fall.
  - Isolation held: one run's reads of the cached go-cmp release were all denied.
- **Decision.** It stays in `-fast`. It helped on the search-heavy development tasks over three rounds, and it was neutral on the held-out set with no cost to passes or spend. The development gain did not carry over, so the claim is "fewer round trips where the agent searches a lot", not "13% faster". To be reviewed by the user.
