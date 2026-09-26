# Fast flow, second design: faster than the default flow on real repositories too

26 September 2026. Branch `feat/scale-bench`. Decisions: [0007](../docs/decisions/0007-fast-flow-at-scale.md), [0008](../docs/decisions/0008-lookup-and-three-moves.md), [0009](../docs/decisions/0009-cross-check.md).

## Result

Same model (`meta/muse-spark-1.3-contributor`), both flows run side by side in each `bench.sh` call, 3 runs per task. These are the final runs, on the code with the screened cross-check (0009):

| Suite | Fast flow | Default flow | Speed-up (mean) |
|---|---|---|---|
| Small fixtures (13 tasks) | 39/39, 23.0 s mean, 16 s median | 39/39, 46.4 s mean, 37 s median | 2.0× |
| Real repositories (6 tasks) | 18/18, 40.7 s mean, 30 s median | 18/18, 58.5 s mean, 50 s median | 1.4× |

- **Speed:** the fast flow was faster on 18 of the 19 tasks.
- **gm-strike-tag:** the exception. Over five runs it ranged from 81 to 96 s for the fast flow and 79 to 111 s for the default flow, so it is level within noise.
- **Evidence:** bug fixes now come with a test that reproduces the report. An independent cross-check tests every change from the task's words and has caught real edge cases the agent's own tests missed.
- **Cost:** the reported cost is an upper bound that counts every raced copy. On the small suite it was $0.0033 a run against $0.0021. At scale, OpenRouter's billed spend has been close to the default flow's (0007).

Provider speed changes a lot through the day. Every comparison here is between flows run in the same window, never across windows.

## What changed since the first fast flow

The first fast flow (research 03) was built on 1 to 5 KB fixtures. On real repositories it was no faster than the default flow, and its reported cost was 19 times higher. Three rounds of fixes, each driven by benchmark traces:

1. **Round 1: make it work at scale (0007).**
   - A snapshot of named code instead of files in path order.
   - `search` and `definition` tools, and whitespace-tolerant edits.
   - Only apply checks count as evidence, and checks run with pipefail.
   - Hedged racing, and honest cost accounting checked against billed spend.
2. **Round 2: remove the remaining steps (0008).**
   - One `lookup` call that fetches many files, definitions and searches.
   - A prompt built on three moves: gather, change, fix.
   - A regression test in the same apply as every bug fix.
   - Whole small files for named code.
3. **Round 3: be at least as right as the default flow (0009).**
   - An independent cross-check, written from the task's words while the agent works.
   - Jev screens a failing cross-check, so false alarms don't cost the agent a step.

## Why it works

- **Latency comes per step, not per token.** On this model every LLM step costs 2 to 4 s, however little it does.
- **The default flow spends most of its steps on deterministic work.** Listing files, reading them one at a time, editing one hunk, running the tests, then writing a summary.
- **The fast flow moves that work into code, or folds it into fewer, larger calls:**
  - Code gathers the named code before the first call.
  - One lookup fetches everything else.
  - One apply changes and verifies.
  - Jev ends the run as soon as the check's output shows the task done.
- **Racing trims per-step latency.** It removes the variance in each step's own time.
- **It fails gracefully.** When the task needs real exploration, as on gm-strike-tag, the flow falls back to looking things up step by step. It is then no slower than the default flow.

## Still open

- **gm-strike-tag.** The model writes a correct implementation within about 15 s, then spends a minute learning goldmark v2's test API to write tests. That's a knowledge gap in the model, not a flow problem.
- **Candidates for API-learning tasks:**
  - Attach the definitions of names a failing compile check calls undefined.
  - Run targeted tests while iterating, and the full suite once.
- **Coverage.** Six scale tasks and one model. The next step is a bigger, public benchmark in the SWE-bench style.
