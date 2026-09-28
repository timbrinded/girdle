# 0014 ast-grep with Jev: structural facts, judged meaning

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

ast-grep parses code and shell commands with tree-sitter grammars. That lets code read structural facts that it would otherwise guess with regular expressions: where a function starts and ends, what calls what, which commands a shell line runs, and whether a mention is code, a comment or a string. The project's rule is that code reads facts and Jev judges meaning. This record covers each idea tried with the two together, what it measured, and what was kept.

Girdle calls the `ast-grep` binary, as it already calls `rg`. Without it, each feature degrades. The tripwire asks Jev about every command, and the structural context and unused-code facts are skipped. The scripts are in `research/experiments/astgrep/`.

## Kept

### A tripwire under every shell command (on by default)

- **The gap.** The North Star requires stopping catastrophic actions: deleting outside the project, force-pushing a shared branch, or sending secrets off the machine. It also requires a hard floor in code under Jev. Neither existed.
- **Facts.** ast-grep's bash grammar finds every command in a line, including those in pipelines, lists, `$(…)`, `bash -c` strings and `eval`. It unwraps `sudo`, `env`, `timeout` and the like. Inline Python and JavaScript (`python3 -c`, `node -e`) are parsed with their own grammars for calls that spawn processes, delete files or use the network.
- **Floor.** Code always blocks these, whatever Jev says:
  - recursive or glob deletes outside the project and the temp directories, or of home, the root, or a parent of the project;
  - disk erasers;
  - force-pushing or deleting main, master or another shared branch, or the current branch unnamed;
  - a network send in the same line as a secret, such as a key file, `.env`, a `*KEY*` or `*TOKEN*` variable, or a bare `env`.
- **Fast path.** A command that deletes, pushes, sends and writes nothing outside the project runs at once.
- **Jev.** Everything else gets one request with four risk questions and "did the user ask for this?". It blocks when a risk reaches 0.5 and the user didn't clearly ask. It fails closed when Jev can't be reached. "Damages the system" is asked and logged but doesn't act: it reached 0.45 on a plain `go test`, and it caught nothing the other questions missed.
- **A block ends the turn and hands the run to the user,** rather than letting the agent look for a way round it.
- **Measured:**

  | Command set | Blocked | Ran | Asked Jev |
  |---|---|---|---|
  | Catastrophic commands and evasions, 39 | 39 (35 by the floor) | 0 | 4 |
  | Safe commands that look dangerous, 31 | 0 | 31 | 5 |
  | Every command the benchmark agents ran, 3,727 | 0 | 3,727 | 17 |

  Live, over 44 benchmark runs, it made no decisions at all: every command took the fast path.

### Leftover checks for renames and removals (`-leftovers`, on in `-fast`)

- **The failure.** Every failed rename in the benchmark logs left the old name behind, in a comment or a test's message. Every passing one left none:

  | Rename runs, final code | Old name left | Clean |
  |---|---|---|
  | Failed | 6 | 0 |
  | Passed | 0 | 304 |

- **How.**
  1. When a request starts, Jev judges, for each name and file the request mentions, whether it should be gone by the end. It asks: renamed, replaced or removed? Should a file's unused code go?
  2. Before an early stop, and at turn end, code finds the remaining mentions of those names. That's a text search, so comments and strings count.
  3. Jev judges which mentions the task really needs changed, so that a changelog line recording the rename can stay.
  4. For files to prune, ast-grep lists functions that no code references. It tells code from comments and strings, which a text search can't.
  5. The agent gets a nudge naming the exact lines, at most twice per request.
- **Measured.**
  - Replayed on the six failed runs, all six would have been nudged, each naming the leftover lines. Jev marked `calcTotal`, not `computeTotal`, as meant to go.
  - On the removal task, the unused-code fact named exactly the three unused functions, and flagged none of the 176 passing runs.
  - Live, the rename and removal tasks passed 15 of 15 runs at minimal effort with and without the check, so there was nothing to catch.
  - On five unrelated tasks, the fast flow with the check passed 10 of 10 runs with no leftover nudges. A false nudge would come from Jev wrongly marking a name as meant to go.
- **Cost.** The intent question runs alongside routing. At a stop, the text search runs only for names Jev marked, and Jev is asked again only if mentions remain.

### Snapshots that are the same every time

`tools.Search` returned ripgrep's matches in whatever order its threads found them. So the uses in a large repository's snapshot, which is the first message of every request, changed from run to run, and the prompt cache missed from that point on. Search now sorts by file and line. On the more-itertools tasks, the cached share of the first call's input went from between 16% and 39% to between 65% and 98%.

## Built, kept as an opt-in flag

### Structural context around the named code (`-structure`)

- **The cost it targets.** After prefetch, runs on real repositories still spent lookups on line ranges of large files. The agents looked up the test class around the named code, the functions it calls, its type stubs and export list, and the test helpers that sibling tests use.
- **How.** ast-grep splits every file into top-level units with exact line ranges, each with the comment above it. The snapshot then adds, within 16 KB and in this order:
  1. the tests that use a named definition;
  2. its other declarations;
  3. the functions it calls;
  4. the helpers used only by the tests beside it, ranked first because nothing outside tests uses them;
  5. the units in its own file that use it.
- **Offline.** The share of looked-up units already in the snapshot rose from 17% to 68%.
- **Live, over two rounds on the real-repository suite:**

  | Round | Fast flow | With `-structure` |
  |---|---|---|
  | 1 | 42.2 s, 3.3 lookups | 36.1 s, 1.5 lookups |
  | 2 | 28.3 s, 1.8 lookups | 33.6 s, 1.7 lookups |

  Pooled, lookups fell 37%, but time didn't change: the first call is about 1 s slower with 16 KB more to read. Only mi-unique-window improved in both rounds.
- **Jev didn't help here.** Asked, unit by unit, whether the agent would need it, Jev ranked test utilities low (about 0.3) and parser internals high. The structural order alone covered as much.

## Measured and not built

| Idea | Why not |
|---|---|
| ast-grep outlines for prefetch's per-file questions | No better than the regex outline: 10 of 21 in the top five either way |
| Edits that name a definition, instead of quoting its old text | Quoted old text is 16% of apply input, and 12 of 454 applies failed to match |
| Exact signatures for the cross-check writer | Only 49 of 939 cross-checks failed on their own build errors |
| Richer compile-error hints (signatures and usage examples, ranked by Jev) | The long recoveries are almost all one task, whose test helpers `-structure` already shows |
| Detecting weakened tests from the syntax tree | One run in 3,227 removed assertions from an existing test |
