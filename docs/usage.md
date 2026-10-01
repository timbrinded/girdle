# Usage

Girdle has an interactive terminal UI and a headless mode. Both use the same session kernel and write an event log. [Configuration](configuration.md) lists the CLI flags and provider settings.

## Interactive requests

Run `bin/girdle -C /path/to/project`, enter a request and press Enter. A session retains conversation history across requests. Ctrl+C cancels the active request; Ctrl+C while idle exits. Page Up and Page Down scroll the output.

Model and effort changes take effect on the **next request**, including changes queued while a request runs. A model switch converts the existing history for the new provider by dropping earlier reasoning and serializing parallel tool calls with their results.

## Models and reasoning effort

The model picker is available with the OpenRouter provider. It fetches the models your key can use and offers those that support tool calls. If that request fails, it tries the public catalogue, which can include models your account cannot use. If both requests fail, the TUI retains its available cached catalogue.

| Control | Action |
| --- | --- |
| Ctrl+L or `/model [search]` | Open and search the model picker |
| Enter in the picker | Select the highlighted model for the next request and add it to your models |
| Ctrl+F in the picker | Add or remove the highlighted model from your models |
| Ctrl+S in the picker | Save the highlighted model and current effort as the defaults for new sessions |
| Tab in the picker | Open the effort choices for the current model |
| Escape | Close the picker |
| Ctrl+P outside the picker | Cycle your models |
| Shift+Tab outside the picker | Cycle `auto` and the efforts the current model accepts |
| `/model <id> [effort]` | Select a known model directly, optionally setting effort |
| `/effort` or `/effort <level>` | Open the effort picker or set effort |

Your models appear first; other models are grouped by provider. Each row shows catalogue pricing, context size and supported efforts. Search words match model IDs and names.

Removing the active or default model promotes the most recently picked remaining model. The last saved model cannot be removed. When the TUI has catalogue information showing a model is unavailable, it tries a remaining available model and reports the change. Headless runs do not load or refresh that catalogue.

`auto` lets Jev choose **reasoning effort for the selected model** per request. Girdle does not automatically choose a different model for each step. A pinned effort is fitted to the model's supported choices when that information is available: the requested choice, otherwise the lowest above it, otherwise the highest below it. `none` is used only when requested. A known model without an effort setting receives no effort parameter.

For headless runs, `-reasoning` sets the fallback effort. Add `-no-route` to pin it. `-reasoning` alone keeps automatic routing enabled. Headless runs send the requested effort without reading the TUI's catalogue, so it must suit the model.

## Headless runs

```bash
bin/girdle -fast -C /path/to/project -p "fix the failing test" -timeout 10m
bin/girdle -C /path/to/project -p "explain the parser" -json > events.jsonl
```

Place flags before positional arguments. `-p` supplies the request; `-seed` can instead supply a seeded conversation for a gate or experiment. A supplied `-p` adds a request after the seed; without it, the seeded conversation resumes. See [configuration](configuration.md#run-and-output).

| Exit | Meaning |
| --- | --- |
| `0` | The harness judged the request complete (`done`) |
| `1` | Configuration, provider or execution error |
| `2` | The request needs user input or a decision (`needs_user`) |
| `130` | The request was cancelled, including the headless timeout |

The terminal result describes the harness's decision. Check the diff and verification output to establish whether the task is correct. With `-no-checkpoints`, an ordinary LLM turn end is treated as `done`.

`-json` emits one JSON event per line to stdout and omits streaming text deltas. Setup errors and notices can still appear on stderr. The persistent event log is written separately even when stdout is redirected.

## Session logs

By default, a new session writes to `$XDG_STATE_HOME/girdle/sessions/<timestamp>-<pid>.jsonl`, or `~/.local/state/girdle/sessions/` when `XDG_STATE_HOME` is unset. Use `-log /path/to/session.jsonl` to choose a path. An existing file is appended to.

The event definition is [internal/kernel/events.go](../internal/kernel/events.go). Each event contains `time`, `session` and `type`, plus fields relevant to that type.

| Events | What they record |
| --- | --- |
| `session_start`, `settings` | Model, effort and resolved feature settings |
| `user_message`, `assistant_text`, `reasoning` | Requests, replies and available reasoning summaries |
| `tool_call`, `tool_result` | Tool name, call ID, input and clipped output |
| `step`, `race`, `turn_end` | Timing, usage and estimates for losing raced copies |
| `decision`, `route`, `heartbeat`, `tripwire` | Checkpoint state, answers, policy rule, latency and Jev model when returned |
| `snapshot`, `crosscheck`, `compact`, `leftovers`, `reproduce` | Evidence and outcomes for the corresponding features |
| `nudge`, `error`, `run_end` | Continuation messages, errors and terminal outcome with reason |

Streaming `text_delta` events reach the TUI but are excluded from both JSON output and the persistent log. Concurrent event writes are serialized within the session.

For example, inspect a log with Python's standard library:

```bash
python3 - /path/to/session.jsonl <<'PY'
import json
import sys

with open(sys.argv[1]) as events:
    for line in events:
        event = json.loads(line)
        if event["type"] in {"decision", "tripwire", "run_end"}:
            print(json.dumps(event, indent=2))
PY
```

Logs can contain source code, prompts, commands and secrets present in tool output. Girdle does not redact them. Review them locally and redact before sharing; see [security](../SECURITY.md).
