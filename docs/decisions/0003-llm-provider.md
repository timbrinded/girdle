# 0003 LLM provider and model

**Date:** 2026-09-26 · **Segment:** 1

**Decision:** The LLM goes through `charm.land/fantasy` v0.45.2 with its OpenRouter provider. The default model is `meta/muse-spark-1.3-contributor` ($0.10/M in, $0.20/M out), overridable with `-model` or `GIRDLE_MODEL`, with reasoning effort `medium`. Fantasy runs the inner step loop (LLM → tools → LLM until no tool calls). Girdle owns everything between turns.

**Why:** It was the user's choice, for cost. One OpenRouter key reaches cheap and strong models, which later makes model routing a config change.

**Caveat:** The Contributor tier lets Meta train on prompts and responses, and is limited to 100 requests per minute. Don't point Girdle at private code with this default.
