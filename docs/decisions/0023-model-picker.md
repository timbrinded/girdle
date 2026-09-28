# 0023 Model picker and reasoning effort

**Date:** 2026-09-28 · **Branch:** `feat/model-picker`

- **Decision.** The TUI gets a model picker for OpenRouter models (`ctrl+l`), and the reasoning effort can be changed on its own (`shift+tab`). Both keys are free in the input box, and the status line and prompt name them.
- **What OpenRouter says is read as fact.** Its catalogue (`GET /api/v1/models`, no key needed) gives each model's accepted efforts (`reasoning.supported_efforts`), whether it takes tool calls, its price and any withdrawal date. The TUI fetches it at start and caches it in `~/.cache/girdle/openrouter-models.json`. Headless runs only read the cache.
  - The add search offers only models that take tool calls, since Girdle can't work without them.
  - Each request asks for the nearest effort the model accepts: the one asked for if accepted, else the lowest above it, else the highest below. Rounding up keeps a request from getting less reasoning than it was routed to. `none` turns reasoning off, so it's only used when asked for.
  - A model with no effort setting is sent none. A model the catalogue doesn't know, or any model with `-provider zen`, is sent the effort as asked, as before.
- **The list and defaults.** `~/.config/girdle/models.json` holds the user's models and when each was last picked. `d` in the picker saves the current model and effort setting as what new sessions start with.
  - The start model is `-model` or `GIRDLE_MODEL`, then the saved default, then Space Bunny. This applies to headless runs too. The benchmark always passes `-model` and `-reasoning`, so saved defaults never reach it. `bench/gate.sh` passes neither, so it follows them.
  - The start effort is `-reasoning` or `-no-route` if either is given, then the saved setting, then auto.
- **Removal promotes the last picked model.** Removing the model in use switches to the most recently picked model still on the list. Removing the default makes that model the default. A model OpenRouter stops listing is treated the same way, at start from the cache and again when the fresh catalogue arrives. It stays on the list, marked as no longer listed. The last model on the list can't be removed.
- **Auto effort is Jev's route.** The effort setting is `auto`, where Jev routes each request as before, or one pinned effort.
- **Jev now routes even a pinned effort.** Before, `-no-route` skipped the route entirely. That also dropped its "does the request ask for tests?" answer, so early stop could end a run before the tests it asked for were written. A pinned effort's route runs alongside the first LLM call, which starts at once on the pinned effort, so it adds no wait. The route event now records the effort used (`effort`) beside Jev's choice (`route.effort`).
- **Settings change between requests.** The TUI hands new settings to `Session.Configure`, and the next request applies them. A change made while a request runs waits for it. The log gets a `settings` event when they apply. Background work still finishing the last request keeps the settings it started with.
- **A failed route falls back to `-reasoning`, not a fixed medium.** `-reasoning` defaults to medium, so nothing changes unless it's set.
