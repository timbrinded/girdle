# 0007 Fast flow at scale: named-code snapshot, code tools, hedged racing

**Date:** 2026-09-26 · **Branch:** `feat/scale-bench`

**Context:** The 13-task benchmark uses fixture repos of 1 to 5 KB, which fit whole in a snapshot. A scale suite (`bench/scale`) puts six tasks on real repositories at a pinned commit:
- goldmark: 127 files, 1.6 MB, and a 10 s test suite.
- more-itertools: a 175 KB, 5,600-line module, and a 7 s test suite.

On that suite the fast flow passed every run, but it was no faster than the default flow and reported about 19 times the cost.

**What broke, and the decision for each:**
- **The snapshot filled its 64 KB budget in path order, mostly with docs.** A repository too large to include whole now gets its file list plus, in priority order:
  - the repository's agent instructions (`AGENTS.md`, `CLAUDE.md`)
  - files the request names in backticks
  - the definitions and uses of the code names the request puts in backticks
  - the test files beside that code

  Lookups are exact: a code span is the user's explicit reference to code.
- **Jev can't pick relevant files from paths.** One request with 127 relevance Nouls took 0.4 s but ranked `text/value.go` first for a strikethrough task, and `docs/toctree.rst` first for `sliding_window`. Paths don't carry that information, so the snapshot uses exact lookups instead.
- **The model spent dozens of steps on grep and paged reads.** Two new fast-flow tools:
  - `search`: ripgrep, capped at 80 matches and grouped by file.
  - `definition`: the source of a Go, Python or JS/TS function, method, class or type, with its comments and line range.
- **Every grep after an edit counted as evidence and cost a Jev call.** With apply, only an apply's check is evidence.
- **Checks such as `go test ./... | tail -20` hid failures.** apply runs checks with `pipefail`.
- **Edits failed on tabs versus spaces.** An `old_text` that matches exactly one run of lines, with whitespace ignored, now applies, re-indented to the file. An apply with no changes just runs its check.
- **Racing tripled every call.** Racing now hedges: a request's first call races every copy at once, and later calls start an extra copy only after 3 s without an answer.
- **Cost reporting overstated racing about 2.6 times.** Cancelled copies share the prompt cache, so each loser is now priced like its winner, which is still an upper bound. OpenRouter's billed spend on the suite, over 12 runs per flow, was $0.0055 a run for the fast flow and $0.0048 for the default flow.
- **git lists nothing in a directory an enclosing repository ignores.** The file listers now walk the tree instead.

**Why:** Each fix came from a failure seen in the benchmark traces. None of them decides meaning with a heuristic. They read facts (code spans, exit codes, file names by convention), or they make tools more tolerant of how models actually write input.

**Still open:** Tasks that need the model to learn an unfamiliar API stay slow. gm-strike-tag took 20 to 40 exploration steps, one tool call per step, while the prompt grew to 60k tokens. Next candidates are relevance-judged pruning of old tool output (segment 4), and getting the model to batch its lookups.
