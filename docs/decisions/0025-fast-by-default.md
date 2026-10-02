# 0025 The fast flow is the default

**Date:** 2026-10-01 · **Branch:** `feat/tui-polish`

- **Decision.** Girdle runs the fast flow unless told not to. `-boring` turns it off, as do `-fast=false` and `GIRDLE_BORING=1`. A flag beats the environment, and `-fast -boring` together is an error. `-fast` still works, so existing commands and scripts are unchanged.
- **Why.** The fast flow is what Girdle is for: it was about twice as fast as the old default on the recorded warm Muse Spark suites, at the same observed pass rate ([stage summary](../../research/06-optimisation-stage-summary.md)). A first-time user should get it without knowing to ask.
- **Cost.** The fast flow races three copies of a request's first LLM call and hedges later calls after three seconds, so it can use more tokens and cost more than the boring flow. Its cross-check also makes background LLM calls. The TUI's welcome screen says which flow is running and how many copies race.
- **Benchmarks.** The `girdle` and `girdle-nojev` agents in `bench/run-one.sh` now pass `-fast=false`, so they still measure the boring flow. They use `-fast=false` rather than `-boring` because older builds compared with `@label` don't know `-boring`. Results recorded before this change call the boring flow "the default flow".
