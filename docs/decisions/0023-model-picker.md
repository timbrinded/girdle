# 0023 Model picker and reasoning effort

**Date:** 2026-09-28 · **Branch:** `feat/model-picker`

- **Decision.** The TUI gets a model picker over every OpenRouter model the user's key can use, and a reasoning effort that changes independently of the model. It follows Pi, OpenCode and Grok Build, which were each tried side by side in tmux:

  | Action | Girdle | Borrowed from |
  |---|---|---|
  | Open the picker, search focused | `ctrl+l`, or `/model [search]` | Pi's `ctrl+l`; `/model` in all three |
  | Switch straight to a model | `/model <id> [effort]` | Grok Build |
  | Set the default | `ctrl+s` in the picker | Pi |
  | Add or remove "your models" | `ctrl+f` in the picker | OpenCode's favourite key |
  | Cycle your models | `ctrl+p` | Pi |
  | Step the effort | `shift+tab` | Pi |
  | Pick an effort from a list | `/effort [level]`, or `tab` in the picker | Grok Build |
  | Model and effort shown | Right of the status line | Pi's footer |
  | Key reminder | A row under the input, which lists the commands while one is typed | Grok Build |

  The row drops its least important keys first when the terminal is narrow. `ctrl+p` no longer moves up a line in the input; the up arrow still does.
- **The list is the one the key can use.** The TUI fetches `GET /api/v1/models/user` with the OpenRouter key. OpenRouter filters that list by the account's provider preferences, privacy settings and guardrails. If it fails, the TUI falls back to the public `/api/v1/models`. The list is cached in `~/.cache/girdle/openrouter-models.json`. Headless runs don't read the cache, so what a benchmark run sends never depends on when the TUI last cached the list on that machine.
  - The picker offers only models that take tool calls, since Girdle can't work without them: 390 of 458 on 2026-09-28.
  - "Your models" come first, then every other model, grouped by provider with the newest first in each.
  - Each search word must match the ID or name. It can match as a substring, which scores best at the start of a part (`claude` in `anthropic/claude-opus`), or as a tight subsequence, so `musespk` finds `muse-spark`. Ties go to your models, then to newer models, then to shorter IDs, so a base model comes before its `:batch` variant. Pi ranks Mistral first for "opus 4"; this ranks only Opus.
- **What OpenRouter says about reasoning is read as fact.** Its docs define `reasoning.supported_efforts`:
  - A list means only those efforts.
  - `null` means every effort.
  - An absent field means the model exposes no effort choice.

  A model that has no list but takes the `reasoning` parameter, such as Claude Opus 4.5, still gets any effort, because OpenRouter turns it into a thinking budget. So it is sent the effort as asked, as before this branch. Only a model that doesn't take the parameter at all is sent none.
  - Each request asks for the nearest effort the model accepts: the one asked for if accepted, else the lowest above it, else the highest below. Rounding up keeps a request from getting less reasoning than it was routed to. `none` turns reasoning off, so it's only used when asked for.
  - A model the catalogue doesn't know, or any model with `-provider zen`, is sent the effort as asked, as before.
- **Your models and the defaults** live in `~/.config/girdle/models.json`, with when each model was last picked. Picking a model adds it to your models. Starting on a model doesn't, so a one-off `-model` never becomes what later sessions start on. The model in use is shown with your models until it's saved.
  - The start model is:
    1. `-model` or `GIRDLE_MODEL`;
    2. otherwise the saved default;
    3. otherwise, as in OpenCode, the most recently picked model the key can still use;
    4. otherwise Space Bunny.

    This applies to headless runs too. The benchmark always passes `-model` and `-reasoning`, so saved models never reach it. `bench/gate.sh` passes neither, so it follows them.
  - The start effort is `-reasoning` or `-no-route` if either is given, then the saved setting, then auto. An unknown `-reasoning` is an error.
- **Removal promotes the last picked model.** Removing the model in use from your models switches to the most recently picked of the rest. Removing the default makes that model the default. A model the key can no longer use is treated the same way, at start from the cache and again when the fresh list arrives, and `ctrl+p` passes over it. The last of your models can't be removed.
- **Auto effort is Jev's route.** The effort setting is `auto`, where Jev routes each request as before, or one pinned effort.
- **Jev now routes even a pinned effort, without anything waiting on it.** Before, `-no-route` skipped the route entirely. That also dropped its "does the request ask for tests?" answer, so early stop could end a run before the tests it asked for were written.
  - A pinned effort's route runs in the background. The LLM calls never wait for it, so a slow or unreachable Jev costs a pinned request nothing.
  - Only the step end, which is about to ask Jev itself, waits for the answer. The request's end logs it if it has come.
  - A speculative first call takes its route the same way: one pending route, taken by whichever needs it first (`kernel.takeRoute`).
  - The route event now records the effort used (`effort`) beside Jev's choice (`route.effort`).
- **Settings change between requests.**
  - The TUI hands new settings to `Session.Configure`, and the next request applies them. The log gets a `settings` event when they apply.
  - A change made while a request runs waits for it. The status line then shows both the running settings and the queued ones ("… → next …").
  - Background work still finishing the last request keeps the settings it started with.
- **A new model gets a portable history.** When the model changes, the kernel rewrites the conversation into the form every chat API accepts:
  - reasoning from earlier assistant messages is dropped, since it is the old model's own and often encrypted or signed;
  - parallel tool calls become one call per turn, each followed by its result.

  tmux testing found the need. After a Muse Spark request, Space Bunny failed every time with "JSON error injected into SSE stream". Replaying the request showed its provider returns 502 for any history with two or more parallel calls, and succeeds once they are split.
- **A failed route falls back to `-reasoning`, not a fixed medium.** `-reasoning` defaults to medium, so nothing changes unless it's set.
