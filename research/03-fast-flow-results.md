# Fast flow results: the same work in one or two LLM steps

26 September 2026. Branch `perf/fast-flow`. Decisions: [0005](../docs/decisions/0005-fast-flow.md), [0006](../docs/decisions/0006-racing-and-speculative-routing.md).

## Where the time went

In segment 1's benchmark, model time was 97% of a Girdle run. A typical run took about 8.6 LLM steps:
- about 3 exploring (listing and reading files)
- about 3 making one edit each
- 1 or 2 running the tests
- 1 writing the reply

Every step pays 1 to 3 s before its first token. Edit steps were the slowest, at about 10 s each, because output streams at about 150 to 400 tokens a second.

## What `-fast` changes

1. **Snapshot.** The repository's files go out with the request, so nothing is explored.
2. **One apply.** A single tool call makes every change and runs a check that proves the whole task.
3. **Early stop.** When the check exits 0, Jev reads the tool results, and a confident "complete" ends the run with no reply step.
4. **Racing.** Each LLM call is sent three times, and the first complete answer wins.
5. **Speculative routing.** The first call starts on low effort while Jev routes.
6. **Small edits.** The prompt asks for small edits, compact tests, and checks that never hide their exit code. The apply tool accepts a common malformed input instead of rejecting it.

## Result

Same model (`meta/muse-spark-1.3-contributor`), 13 tasks with hidden tests, headless. Provider speed changes through the day, so every comparison below ran side by side in one `bench.sh` call.

**Final run (fast-final3), 5 reps:**

| | Pass | Mean | Median | p90 | LLM steps per run | Cost per run |
|---|---|---|---|---|---|---|
| Fast flow | 65/65 | 15.5 s | 10 s | 36 s | 1.5 | $0.0025 |
| Default flow | 65/65 | 35.9 s | 30 s | 73 s | 8.0 | $0.0019 |

**Three-way run (fast-final2), 5 reps, before the apply robustness fixes:**

| | Pass | Mean | Median | p90 | Cost per run |
|---|---|---|---|---|---|
| Fast flow | 64/65 | 16.1 s | 12 s | 36 s | $0.0027 |
| Default flow | 64/65 | 37.5 s | 28 s | 82 s | $0.0022 |
| Pi | 65/65 | 40.7 s | 30 s | 89 s | $0.0030 |

- **The fast flow is 2.3 to 2.5 times faster than the default flow and Pi, and faster on every task in both runs.** The gap is largest on short tasks. go-three-bugs took 6 s against 25 s, and py-dedupe 6 s against 21 s.
- **Reliability held.** Over both runs each Girdle flow passed 129/130. The two misses were js-slugify spec edge cases, one per flow, where the model's own tests agreed with its wrong code.
- **Cost is an upper bound.** Racing's cancelled copies are counted as if each used as many tokens as the winner. At that bound the fast flow costs about 30% more than the default flow and still less than Pi.

**Where the fast flow's 15.5 s goes (fast-final3, mean per run):**

| Part | Time |
|---|---|
| LLM (raced) | 14.5 s |
| Tools, mostly the check | 0.7 s |
| Jev step-end check | 0.3 s |
| Jev routing | 0 s: it runs alongside the first LLM call |

51 of 65 runs (about four in five) finished in one LLM step. The rest need a fix-up after a failed check, or a second step after Jev reads the task as not yet complete.

## What didn't help

- **Reasoning `none`.** This model rejects it.
- **Reasoning `minimal`.** Faster, but it fails spec-heavy tasks.
- **Racing 5 copies.** It was slower than racing 3 and less reliable. The first of 5 to finish is often the copy that reasoned least.
- **Splitting the implementation and the tests into two concurrent calls.** The implementation half does most of the reasoning, so nothing was saved.

Details are in the decision records.

## What's left

Nearly all of a fast run is one model call. The remaining levers are the model's own speed:
- time to first token, which grows with hidden reasoning
- output speed
- how often the first answer passes its own check

A faster provider or model would move the numbers most. The benchmark kept the model fixed to compare flows fairly.

## Caveats

- 13 small, well-specified tasks, one model, headless.
- The step-end threshold (`complete` ≥ 0.8) and the new low-effort cut (1.85) were tuned on this benchmark.
- With racing on, the TUI shows each step's text at once rather than streaming it.
