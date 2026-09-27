# 0017 OpenCode Zen as a second provider

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

- **Decision.** Girdle can reach models through OpenCode Zen as well as OpenRouter, with `-provider zen` and `OPENCODE_API_KEY`. `-jev zen` calls Zen's `jev-1.13-free` instead of TypeSafe's pinned `jev-1.13.0`. The benchmark takes `BENCH_PROVIDER=zen`.
- **Why.** Zen offers free models for limited periods:
  - LongCat 2.5 Preview (`longcat-2.5-preview-free`), free for two weeks from 26 September 2026. Its provider retains no data and doesn't train on it, so it's fit for private code.
  - Muse Spark 1.3 Contributor (`muse-spark-1.3-contributor-free`), the model every result so far used. It trains on prompts.
  - Jev 1.13.
- **Not a free lunch on OpenRouter.** OpenRouter only has LongCat 2.0, at $0.30 per million input tokens and $1.20 per million output, three to six times Muse's price there.
- **How.** Zen is OpenAI-compatible. LongCat uses chat completions, and the Muse models use the Responses API, so Girdle uses Fantasy's `openaicompat` provider with the Responses API switched on for `muse-*` models. The reasoning effort is set in both APIs' options, and each model reads its own.
- **Before relying on it.**
  - A different main model needs its own baseline on the development half.
  - Jev's thresholds were tuned on Muse transcripts, so they need a check against LongCat's.
  - Compare `jev-1.13-free` with the pinned `jev-1.13.0` on replayed decisions before using it for tuned checkpoints.
