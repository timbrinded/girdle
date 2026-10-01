# 0021 Space Bunny Alpha is Girdle's default model

**Date:** 2026-09-28 · **Branch:** `feat/scale-bench`

- **Decision.** Girdle's default model is now `stealth/space-bunny-alpha` on OpenRouter. It replaces Muse Spark 1.3 Contributor. The user chose it.
- **Why.**
  - It is free for now, where Muse cost about $0.04 a run on the hard suite.
  - Every fast-flow change since decision 0018 was measured and tuned on it: the prefetch data skip, grep context, and the ideas cut along the way. None of them has run on Muse, so Muse is no longer the model the fast flow is known to work with.
- **Caveats.**
  - It is an anonymous "stealth" model. Its provider may log prompts, so it suits public code only. For private code, pass `-model` or set `GIRDLE_MODEL` to a model whose provider data policy suits the project, and check Jev's policy separately.
  - It may be withdrawn without notice. If it disappears, the default must change again, and the fast flow's thresholds need re-checking on the new model.
  - Reasoning can't be turned off. `-reasoning none` is rejected by the provider, as it was for Muse.

**Documentation clarification, 2026-10-01.** The original private-code example named Muse Spark Contributor. [Decision 0003](0003-llm-provider.md) records that tier as allowing training on prompts, so that example has been removed. The model-selection decision and measurements above are unchanged.
