# Configuration

CLI flags configure a session. OpenRouter model choices and effort defaults can also be saved in the TUI. Girdle reads keys and provider overrides from the environment; it does not load `.env` files.

## Providers

The LLM provider and Jev provider are independent choices.

| Selection | Credentials | Behavior |
| --- | --- | --- |
| `-provider openrouter` (default) | `OPENROUTER_API_KEY` | Uses the OpenRouter model ID and enables the TUI's model picker |
| `-provider zen` | `ZEN_API_KEY`, falling back to `OPENCODE_API_KEY` | Uses OpenCode Zen; an explicit model is required via `-model` or `GIRDLE_MODEL` |
| `-jev typesafe` (default) | `TYPESAFE_API_KEY` | Defaults to TypeSafe's System One API and `jev-1.13.0`; URL/model environment overrides apply |
| `-jev zen` | `ZEN_API_KEY`, falling back to `OPENCODE_API_KEY` | Calls Zen's Jev endpoint with `jev-1.13-free` |

For example, keep the LLM on OpenRouter and send Jev requests through Zen:

```bash
bin/girdle -jev zen -C /path/to/project
```

With `-provider zen`, select a currently available model suitable for use outside OpenCode. The implementation rejects a missing model ID and notes that Zen's free LLMs are intended for OpenCode. Availability, quotas and prices belong to the provider; check [Zen's documentation](https://opencode.ai/docs/zen/) before choosing. The recorded Zen experiment is [decision 0017](decisions/0017-opencode-zen.md).

Both providers receive context; selecting a different LLM does not change Jev's data handling. See [security](../SECURITY.md).

## Model and effort precedence

The OpenRouter start model is chosen in this order:

1. `-model`, whose initial value comes from `GIRDLE_MODEL`.
2. A saved default model.
3. The most recently picked saved model.
4. The configured fallback, `stealth/space-bunny-alpha`.

The TUI can replace an unavailable choice using cached or refreshed catalogue information. Headless runs read saved choices but do **not** read that catalogue or proactively validate availability.

The start effort comes from explicit `-reasoning` or `-no-route` flags, otherwise the saved effort setting, otherwise `auto`. Automatic routing uses Jev's choice; a failed route uses the configured fallback effort, which defaults to `medium`. `-reasoning high` changes that fallback while keeping automatic routing; `-reasoning high -no-route` pins high effort. Jev still classifies whether tests were requested when effort is pinned.

## Environment and stored files

| Variable | Purpose |
| --- | --- |
| `OPENROUTER_API_KEY` | OpenRouter authentication |
| `TYPESAFE_API_KEY` | Hosted Jev authentication |
| `ZEN_API_KEY`, `OPENCODE_API_KEY` | Zen authentication, in that precedence order |
| `GIRDLE_PROVIDER` | Initial value of `-provider`, otherwise `openrouter` |
| `GIRDLE_MODEL` | Initial value of `-model` |
| `GIRDLE_JEV` | Initial value of `-jev`, otherwise `typesafe` |
| `GIRDLE_JEV_URL` | Base URL for `-jev typesafe`, default `https://api.typesafe.ai`; the client appends `/v1/systemone` |
| `GIRDLE_JEV_MODEL` | Decision model for `-jev typesafe`, default `jev-1.13.0` |
| `XDG_CONFIG_HOME` | Saved models and effort under `girdle/models.json`; default `~/.config` |
| `XDG_CACHE_HOME` | TUI catalogue under `girdle/openrouter-models.json`; default `~/.cache` |
| `XDG_STATE_HOME` | Logs under `girdle/sessions/`; default `~/.local/state` |

The saved list contains model IDs, their last-picked timestamps, and the default model and effort. It contains no API keys. Selecting a model in the picker saves it; starting with a one-off `-model` does not save that choice. Back up `models.json` if you want to preserve choices before resetting them.

## Run and output

| Flag | Default | Meaning |
| --- | --- | --- |
| `-C dir` | `.` | Project working directory |
| `-p text` | Empty | One headless request; otherwise launch the TUI unless a seed is supplied |
| `-provider name` | `openrouter` or environment | LLM provider |
| `-model id` | Saved choice or fallback | Provider model ID |
| `-jev name` | `typesafe` or environment | Jev provider |
| `-reasoning level` | `medium` | Fallback effort: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` |
| `-no-route` | `false` | Pin effort; background request classification can still use Jev |
| `-no-checkpoints` | `false` | Disable the Jev client and completion checkpoints; stop at an ordinary turn end |
| `-max-nudges n` | `4` | Turn-end nudge budget before returning to the user |
| `-max-steps n` | `60` | LLM steps per turn, rather than a whole-session limit |
| `-timeout duration` | `0` | Headless request deadline, such as `10m`; zero has no deadline |
| `-log path` | New session file | Append the event log at this path |
| `-json` | `false` | Headless JSONL events on stdout |
| `-seed path` | Empty | Load a JSON object with `task` and `messages`, each containing `role` (`user` or `assistant`) and `text` |

The [gate seed](../bench/scenarios/announce-then-stop.json) is a concrete `-seed` example. For instance:

```json
{
  "task": "Rename the exported function and update its callers.",
  "messages": [
    {"role": "user", "text": "Rename the exported function and update its callers."},
    {"role": "assistant", "text": "I will inspect the callers first."}
  ]
}
```

The loader converts these text entries into Fantasy messages. A seed is a test input, rather than a way to restore a session from its event log.

## Fast flow

`-fast` enables the features below and sets `-race 3` and `-hedge 3s`. Each explicit flag overrides its fast-flow setting, so `-fast -prefetch=false` leaves prefetch off. All listed Boolean features default to `false` without `-fast`.

| Flag | Effect | Prerequisite |
| --- | --- | --- |
| `-snapshot` | Include repository context with each request | None |
| `-prefetch` | Ask Jev which additional files a large-repository snapshot needs | Snapshot and Jev |
| `-batch` | Use batched `lookup` and `apply` tools and ask for edits plus checks in one step | None |
| `-early-stop` | Judge completion after tool results | Checkpoints and Jev |
| `-stepfan` | Select the broader step-end question and policy set | Takes effect with early stop |
| `-leftovers` | Check mentions of names/files the request wants removed | Jev |
| `-speculate` | Start the first call while effort routing is pending | Routing; acts during auto effort |
| `-crosscheck` | Attempt an independent test in the background | Batch and early stop |
| `-reproduce` | Let `apply` run a regression test with the fix undone | Batch; Jev is needed to claim reproduction |
| `-heartbeat` | Periodically judge loops or drift and nudge | Checkpoints and Jev |
| `-grepctx` | Attach definition context to search results | None |

The kernel disables features whose prerequisites are missing. `session_start` records the resolved values. Cross-check generation may produce no valid test or may not be ready when needed; its event records what happened.

| Other flag | Effective default | Effect |
| --- | --- | --- |
| `-race n` | `1`, or `3` with fast flow | Maximum main-call copies; values below two do not race |
| `-hedge duration` | `0`, or `3s` with fast flow | Delay between launching extra copies after the first call of a request |
| `-compact` | `false` | Prune older tool output judged unnecessary every eight steps; shelved and outside fast flow |

The first main LLM call of each request starts all raced copies together. Later calls launch extra copies only if the earlier copies have not finished before the hedge delay. This affects cost as well as latency.

## Shell access

| Flag | Default | Effect |
| --- | --- | --- |
| `-tripwire` | `true` | Check shell commands before execution; `-tripwire=false` disables the guard |
| `-offline-tools` | `false` | Use macOS `sandbox-exec` to restrict shell network access to localhost |
| `-deny-read regex` | None | Repeatable absolute-path restrictions outside the working directory |

`-offline-tools` and the shell part of `-deny-read` require macOS. On other systems a shell command with either restriction is refused. The LLM and Jev HTTP calls remain online. Direct file tools enforce denied paths separately.

`-no-checkpoints` leaves the tripwire enabled. Commands requiring a Jev judgement are then refused; without ast-grep, that can mean every shell command. See [security](../SECURITY.md) before changing access controls.

Use `bin/girdle -h` for the exact flags of your binary.
