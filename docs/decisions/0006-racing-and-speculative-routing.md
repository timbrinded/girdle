# 0006 Racing LLM calls and speculative routing

**Date:** 2026-09-26 · **Branch:** `perf/fast-flow`

**Decision:**
- **Racing.** `-race N` sends each LLM call N times at once, keeps the first answer to complete, and cancels the rest (`internal/race`). Only the winner's answer reaches Fantasy, so tools run once. `-fast` races 3 copies unless `-race` says otherwise. Answers are buffered, so a raced step shows its text all at once instead of streaming.
- **Cost accounting.** The losers are cancelled when the winner finishes, so each has used at most about the winner's tokens. They are counted at that bound, with their input as uncached, in the run's usage. Reported cost is an upper bound and never flatters racing.
- **Speculative routing.** With `-fast`, a request's first LLM call starts on low effort while Jev routes it, instead of waiting about 0.25 s for the route. The answer is held back until the route arrives. If Jev chose low it is used; otherwise the call is cancelled and started again on Jev's effort. No tool can run on a guess. A restarted call's tokens are not counted.
- **Route cut.** Low effort now covers scores below 1.85, up from 1.7.

**Why:**
- A single step's latency varies a lot between identical requests. Time to first token ranged from 1.1 s to 5.7 s, and reasoning length also varies, so the fastest of 3 copies beats a single draw.
- On the 13-task benchmark, racing 3 cut the mean from 21.8 s to 17.3 s and passed 39/39. The first cost figure was $0.0022 a run against $0.0009 unraced, both at the upper bound.
- **Why 3 copies and not 5.** Racing 5 was slower (18.3 s against 15.4 s) and passed 38/39. The first of 5 answers to finish is often the one that reasoned least, so its checks failed 32 times against 10 for racing 3, and those failures cost extra fix-up steps. Many concurrent streams may also have queued at the provider.
- **Why the lower cut.** js-csv scores 1.76 to 1.8, so it was routed to medium. At low it passed every run in both flows. In segment 1's low-effort experiment it passed 18/18, and in the fast flow 3/3, in 37 s against 48 s at medium. No benchmark task scores above 1.85.
