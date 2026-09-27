# 0019 Optimising the fast flow's time on Space Bunny: where it goes, what was tried

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

The goal: less wall time for `-fast` on the development half of the hard suite, by design (fewer or faster steps), without a lower pass rate or a higher cost. Each idea runs side by side with the current fast flow, and `bench/compare.py` gives the verdict. Held-out tasks are run once, at the end.

## The held-out baseline

The fast flow on Space Bunny, warm, 2 runs per held-out task, before any change here. Only these totals were read.

| | Held-out baseline |
|---|---|
| Passed | 37 of 42 valid runs |
| Mean time | 92 s |
| Median time | 54 s |
| Cost per run | $0.0029 |

The two gj-paths runs are excluded, for the reason below.

## Where the time goes

This is the fast flow on the 11 development tasks, 22 Space Bunny runs, with a mean of 228 s:

| | Per run | Share |
|---|---|---|
| LLM steps | 131 s | 57% |
| Shell commands | 63 s | 28% |
| apply (edits and their check) | 29 s | 13% |
| Jev checkpoints | 1 s | 0.5% |

- **Five commands took 61% of all tool time.**
  - The first was a check that hung after an edit made the expression parser loop. The agent then re-ran it with 400 s, 300 s and 300 s timeouts until the run's limit.
  - The last was a `find /`.
  - The median run has no such tail.
- **LLM step time follows output tokens,** at about 12 s per 1,000. Space Bunny doesn't stream: its first token arrives with the whole answer.
- **Effort isn't a lever.**
  - Reasoning can't be turned off.
  - At minimal, low and medium effort, an exploration step comes out at 90 to 110 tokens either way.
  - Most development runs are already routed to low.
- **By purpose, per run:**

  | Purpose | Steps | LLM time |
  |---|---|---|
  | Reading before the first edit | 4.3 | 45 s, at 10.5 s each |
  | Edits | 5.1 | 39 s |
  | Running code before editing | 1.8 | 21 s |
  | Running code after editing | 3.0 | 15 s |
  | Reading after editing | 2.0 | 10 s |

- **Exploration steps are slow because each one drafts the whole change again.** One run's reasoning redesigned the same lexer change in 8 consecutive lookup steps, at 1,000 to 1,900 tokens each. The earlier reasoning was in the prompt: a probe showed Space Bunny recalls reasoning sent back with a tool call.
- **Edits that don't apply cost a step each, and sometimes whitespace forensics too.** 7 of 21 runs' first apply had an edit whose `old_text` wasn't in the file. Agents then ran `od -c` or `cat -A` to see the spacing.
- **Failures that aren't the agent's cost steps.**
  - 22 of 157 failed checks failed only in files the agent never touched. Examples are `go vet` findings in upstream tests, and a broken `main` package.
  - Agents checked with `git stash` in 16 of 155 runs.
- **Prefetch waited 4 s and found nothing useful on repositories full of test data.** BurntSushi/toml has 600 to 1,000 small files, mostly test data, and 30 that define code. Jev judges about 32 files every 0.3 s. So prefetch always hit its 4 s cap, having judged a random 40 to 60%, and added files such as `cmd/tomlv/main.go`. The first LLM call waits for it.
- **Not levers:**
  - The final verification tail is short: Jev's early stop works.
  - Exact re-reads are rare, at 12 of 202 reads.
  - `old_text` is 5% of output tokens.
  - No free model on OpenRouter was faster than Space Bunny for a reading step. Most were slower or rate-limited.

## A leak through the local disk

- **What happened.** Both held-out gj-paths runs found `~/go/pkg/mod/github.com/tidwall/gjson@v1.19.0` in the Go module cache. That released version already has the `Result.Path` feature the task asks for, and each run made 14 tool calls reading it. Both runs are marked invalid.
- **It's routine.**
  - Space Bunny looks for upstream copies of the code under test in most hard runs. It runs `find / -name block.go -path "*goldmark*"`, and lists the module caches.
  - `-offline-tools` stopped network fetches (decision 0018), but not reads of copies already on disk.
  - Copies on disk include:
    - `go-cmp@v0.7.0`, which has both go-cmp tasks' fixes;
    - the benchmark's clones of other commits;
    - other tasks' warm copies.
- **Decision.** Girdle's `-deny-read <prefix>` blocks reads under a path prefix, except inside the working directory. It applies to shell commands, through `sandbox-exec` rules in which the working directory's allow comes last so it wins, and to Girdle's own read, edit and write. `bench/run-one.sh` denies:
  - `bench/.cache`;
  - `bench/.warm`;
  - the code under test in the module cache and its download cache, found from `go.mod` with the cache's `!` escaping and any `/vN` dropped;
  - any Python package of the repository in site-packages.
- **Checked.**
  - A test reads through the sandbox. The working copy and other dependencies stay readable; the denied copies don't.
  - No development-half run read a leaked copy. That was checked with a scan of every logged run for `pkg/mod`, `bench/.cache`, other warm directories and `find /`.

## The verdict rule, corrected

`bench/compare.py` first compared all runs' times. On round 1, the baseline's failures took 19 to 47 s: they were false "done"s, and they made it look faster. Time is now compared over passing runs only, with pass counts checked apart. The report also gives tool calls and output tokens per passing run, since those don't carry the provider's latency swings. The compute check counts fresh tokens only (uncached input plus output), with cache reads reported apart: they cost about a fiftieth of fresh input.

## Ideas tested

Two rounds, each 2 runs per development task per arm, side by side with the fast flow.

| Idea | What it does | Round 1 | Round 2 | Outcome |
|---|---|---|---|---|
| `-failfacts` | When a check fails only in files the agent didn't touch, or times out, runs it on the original code and reports the result. On a timeout, names what was still running and its CPU time. | 0.87 (0.56 to 1.22), 18 against 15 passes | In the bundle | Cut |
| `-nearmatch` | Applies an edit that matches once with spacing ignored. Otherwise shows the file's closest lines. In replay, 4 of 27 real misses would apply and 20 would get their lines. | 0.87 (0.54 to 1.37), 17 against 15 | In the bundle | Cut |
| `-skipdata` | Prefetch drops files that define no code when there are too many candidates to judge in time | Not run | In the bundle | Kept, as prefetch's standard behaviour |
| failfacts + nearmatch + skipdata | The three together | Not run | 1.04 (0.85 to 1.34), 20 against 18 | See the rows above |
| `-planonce` | One prompt sentence: while gathering, decide only what to read next, and draft the change once. On 12 fixed states, 0.80 of the output tokens (0.53 to 1.15). | Not run | 1.01 (0.79 to 1.28), 17 against 18 | Cut |
| `-structure` | Decision 0014's code around the named code | 0.73 (0.47 to 1.15), 18 against 15 | 1.21 (0.92 to 1.66), 18 against 18 | Removed |
| `-compact` | Jev prunes old tool output every 8 steps | 0.65 (0.47 to 0.87), 15 against 15 | 0.89 (0.69 to 1.14), 14 against 18 | Stays shelved. Pooled, 0.81 (0.65 to 0.97), but 29 against 33 passes. |

Ratios are the candidate's time over the fast flow's, over passing runs, with 95% intervals. Passes are out of 22 each.

- **Round 1's baseline was unlucky.** Its passing runs averaged 21.4 steps, against 13.5 to 15.9 for every other arm. So every idea looked faster in round 1, and nothing held up in round 2.
- **Run-to-run swings of 2 to 5 times** on the same task and arm make effects under about 15% invisible at 2 runs per task.
- **Where the mechanisms fired, they did what they were built for, but no pass or time moved with them.**
  - The pass differences came from runs where neither mechanism fired.
  - Hangs and leftover pre-existing failures were rare in these rounds.
  - Per the no-baggage rule, they are removed. Their measurements are recorded here if hangs become common.
- **Skipping data files is kept.** It is a fact about critical-path time, not a noisy effect. On the toml tasks, the snapshot is ready after 0.5 s instead of 4.1 s, and Jev tokens per run fall from about 261k to 29k. Repositories with fewer than 128 candidate files are unchanged by construction.
- **`-structure` is removed** after failing on both suites, along with ast-grep's `Units` and `Referenced`, which only it used.

## Round 3: show the code instead of letting the agent look for it (`-wide`)

- **Why.** Reading before the first edit costs 45 s a run, and Space Bunny has a 1M-token context. Measured directly, a cached prompt barely slows a step: 81k cached tokens took 3.8 s, 12k took 4.1 s. So `-wide` filled a large repository's snapshot with up to 320 KB of whole code files, in the order Jev's prefetch ranks them.
- **What the snapshots held.** 200 to 330 KB covering what these tasks touch: expr's lexer, parser, compiler and checker, and toml's parser and decoder.
- **Result.** 3 runs per task, side by side:

  | | Fast flow | `-wide` |
  |---|---|---|
  | Time ratio (passing runs) | | 1.02 (0.88 to 1.19) |
  | Passed | 28 of 33 | 29 of 33 |
  | Tool calls per passing run | 15.7 | 12.3 |
  | Output tokens per passing run | 23.8k | 26.9k |

  - Fresh input rose fivefold: the first call's 76k tokens are raced three ways.
- **Cut.** The agent took 22% fewer steps, but thought longer in each, and time didn't move.

## What the rounds show

- **Tools are only 11% of a passing run.** LLM steps are 86%, over rounds 2 and 3. The earlier profile's 41% for tools came from hang tails and benchmark load.
- **Space Bunny's output per passing run held at about 20k to 27k tokens whatever the harness did to its context.**
  - More code shown up front (`-wide`) meant fewer steps and more thinking in each.
  - Pruned context (`-compact`), structural context (`-structure`) and a plan-once prompt (`-planonce`) didn't move output either.
  - Its thinking per task looks roughly fixed. Step time follows output tokens.
  - That leaves time to the model, to the provider's speed, or to spending on copies (racing). None of those is a harness design lever.
- **The kept change, prefetch skipping data files, is confirmed on the held-out set:** results to follow.

## Ideas measured and dropped before a live round

| Idea | Why dropped |
|---|---|
| Restart a run that runs long, as for heavy-tailed searches | Even an oracle cutoff per task saves only 5% (geometric mean over tasks). The run times vary, but their tail isn't heavy. |
| A faster free model for reading steps | Space Bunny was the fastest. The others were slower or rate-limited. |
| Lower effort while exploring | Minimal, low and medium effort gave the same 90 to 110 tokens per step. Reasoning can't be turned off. |
| Attaching the code a failure points at | Only 16% of failed checks were followed by such a read, about 3 s a run |
| Running the test suite speculatively at the start | Only 10 of 66 runs ran the existing tests before editing |
| Re-tuning the step-end stop for Space Bunny | About 4 s a run under a proxy label, and some of those continues were prudent |
| Sending reasoning back in another format | Space Bunny already recalls reasoning that is sent with a tool call |
