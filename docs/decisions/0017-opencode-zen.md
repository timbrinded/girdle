# 0017 OpenCode Zen: its free Jev, not its free models

**Date:** 2026-09-27 · **Branch:** `feat/scale-bench`

- **The hope.** OpenCode Zen announced LongCat 2.5 Preview free for two weeks from 26 September 2026. It also lists Muse Spark 1.3 Contributor, the model every result so far used, and Jev 1.13 as free.
- **The free models can't be used by Girdle.** With a valid key, both free LLMs answer: "OpenCode's free tier can only be used from within OpenCode". Girdle doesn't imitate OpenCode's client to get round that.
- **LongCat isn't cheaper anywhere else.** It is $0.30 per million input tokens and $1.20 per million output, both on OpenRouter (LongCat 2.0) and on LongCat's own platform (2.0 and 2.5 Preview). That is three to six times Muse's price on OpenRouter, so Muse stays the model.
- **Zen's free Jev can be used.** `jev-1.13-free` answers through the API. Compared with the pinned `jev-1.13.0` on 40 logged step-end states with 59 questions each, it matches the pinned model's own run-to-run noise:

  | 59 questions per state | Mean difference | Largest | Decisions flipped at 0.5 | At 0.7 |
  |---|---|---|---|---|
  | Zen free against pinned, 40 states | 0.0115 | 0.23 | 1.2% | 0.9% |
  | Pinned against itself, 20 states | 0.0115 | 0.14 | 0.9% | 0.5% |

  But the free quota didn't last. Within minutes of benchmark load, and prefetch alone sends one request per candidate file, every request got 429 with `Retry-After: 35824`, about ten hours. So the benchmark keeps the pinned TypeSafe Jev by default, and Zen's is opt-in (`BENCH_JEV=zen`, or `-jev zen` for Girdle).
- **The quota exposed a Girdle bug.** The Jev client obeyed any `Retry-After`, so each checkpoint slept for hours and runs hung until the benchmark killed them. The client now waits at most 8 s. A longer requested wait means the quota is gone, so it fails at once, and the checkpoint falls back: the tripwire blocks, and the other checkpoints use their defaults.
- **Plumbing.**
  - `-provider zen` and `-jev zen` read `ZEN_API_KEY`, or `OPENCODE_API_KEY`.
  - `-provider zen` still works for Zen's paid models. It uses chat completions, with the Responses API for `muse-*` models.
  - Zen sits behind Cloudflare, which refuses unidentified clients (error 1010), Go's default user agent included. Girdle's Jev client now identifies itself as `girdle`.
