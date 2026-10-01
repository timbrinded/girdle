# Scale suite: the fast flow on real repositories

26 September 2026. Branch `feat/scale-bench`. Decision: [0007](../docs/decisions/0007-fast-flow-at-scale.md).

## Why

The 13-task benchmark uses fixture repos of 1 to 5 KB. They fit whole in the fast flow's snapshot, so they couldn't show how the fast flow behaves on real code. The scale suite puts six tasks on two real repositories, cloned at a pinned commit and never vendored:

| Task | Repository | What it stresses |
|---|---|---|
| gm-heading-close | goldmark (127 files, 1.6 MB, 10 s suite) | a bug described only by its symptom, in code outside the snapshot |
| gm-strike-tag | goldmark | a feature against an unfamiliar extension API, with tests |
| gm-rename-indent | goldmark | renaming an exported function across 10 files |
| mi-unique-window | more-itertools (175 KB, 5,600-line module, 7 s suite) | a symptom-only bug past line 4,700 of one file |
| mi-window-deque | more-itertools | a symptom that only shows for windows wider than 20, caused in another file |
| mi-argsort | more-itertools | a new function plus its stub, docs and tests |

Bugs are injected with a `setup.patch`. Hidden tests use `zz` names, and `bench/validate.sh` proves every check fails on the starting repo and passes on the reference patch.

## What happened

Everything below is 3 runs per task, with both flows run side by side.

| Run | Fast flow | Default flow | What changed in the fast flow |
|---|---|---|---|
| Baseline | 18/18, 53.3 s | 18/18, 53.6 s | nothing: the fast flow as built on the small suite |
| v2 | 18/18, 49.9 s | 18/18, 58.3 s | snapshot of named code; `search` and `definition`; evidence only from apply checks; whitespace-tolerant edits |
| v3 | 18/18, 50.5 s | 18/18, 59.9 s | pipefail checks; agent files and neighbouring tests in the snapshot; hedged racing |
| Final | 17/18, 48.8 s | 18/18, 55.7 s | snapshot sections in priority order; cost formula |

- **The baseline broke on scale.** The fast flow was no faster, and its reported cost was about 19 times the default flow's.
- **The snapshot was the main cause.** It filled up with goldmark's docs in path order, and it omitted the more-itertools files that mattered.
- **The fast flow's miss in the final run was not the agent.** Goldmark's own timing test failed under benchmark load. The same change passes the full check when run alone. The goldmark checks now skip upstream timing tests.

**Final run, per task (mean seconds):**

| Task | Fast | Default |
|---|---|---|
| gm-heading-close | 60 | 84 |
| gm-rename-indent | 20 | 28 |
| gm-strike-tag | 100 | 87 |
| mi-argsort | 73 | 92 |
| mi-unique-window | 25 | 24 |
| mi-window-deque | 16 | 19 |

- **Medians:** 32.5 s for the fast flow against 43.5 s for the default flow.
- **Noise:** with 3 runs per task, one run moves a task's mean a lot. v3 showed bigger wins on the same code: gm-heading-close 39 s against 87 s, and mi-argsort 48 s against 97 s.
- **Small suite, same final code:** the fast flow passed 39/39 in 16.1 s, against 37.4 s for the default flow. Nothing regressed.

## Cost

My cost reporting priced each cancelled race copy's prompt as uncached. OpenRouter's billed spend shows that overstated it about 2.6 times. Measured on two sequential scale-suite runs of 12 runs each:

| | Billed per run | Reported, old formula | Reported, new formula |
|---|---|---|---|
| Fast flow | $0.0055 | $0.0142 | about $0.0072 |
| Default flow | $0.0048 | $0.0049 | $0.0049 |

At scale the fast flow costs about 15% more than the default flow. On the small suite it now reports slightly less.

## What still works badly

**Tasks that need the model to learn an unfamiliar API.** On gm-strike-tag the model takes 20 to 40 steps, mostly reading and searching one call at a time. By the end its prompt reaches about 60k tokens. The fast flow's one-shot design doesn't help when the model has to discover how the code works first. Candidates:

- Get the model to batch its lookups, for example `definition` taking several names.
- Relevance-judged pruning of old tool output (segment 4). Prompt caching makes large contexts cheap, so pruning is about latency and focus, not cost.

## Caveats

- Six tasks, two repositories, one model, 3 runs per task.
- The injected bugs and the task texts were written by the same agent that built the harness.
- Billed cost was read from the account total, so any other use of the key during the window would count. The default flow's billed and reported costs agree, which suggests the measurement was clean.
