# Usage

Girdle has an interactive terminal UI and a headless mode. Both use the same session kernel and write an event log. [Configuration](configuration.md) lists the CLI flags and provider settings.

## Interactive requests

Run `girdle -C /path/to/project`, type a request in the box at the bottom and press Enter. Ctrl+J (or Shift+Enter, where the terminal reports it) starts a new line, and the box grows to fit. A session retains conversation history across requests. Ctrl+C cancels the active request; Ctrl+C while idle exits. Page Up, Page Down and the mouse wheel scroll the output; while you are scrolled up, new output doesn't move the view, and the status line says there is more below.

The transcript shows your requests, the agent's replies (rendered Markdown), a summary of its reasoning for each step, each tool call with its result, and Jev's decisions. Ctrl+O shows reasoning and tool output in full. The status line shows what a running request is doing and how long it has taken, then how the last one ended and the session's token use. The TUI follows the terminal's light or dark background.

## Carrying on a conversation

Each conversation is kept in its session log, so a later run can pick it up in the same project directory:

| Command | Action |
| --- | --- |
| `girdle -c` | Open the TUI on the latest conversation in this directory |
| `girdle -resume <id>` | Open the TUI on the conversation with this ID, or a unique start of it |
| `/resume [search]` in the TUI | List this directory's earlier conversations, newest first, and carry on the one you pick |
| `girdle -c -p "…"`, `girdle -resume <id> -p "…"` | Add a headless request to that conversation |

The TUI shows the conversation as it ran, then the next request continues it with the same history the LLM had. A headless run ends by printing the command that carries its conversation on. The conversation keeps its ID and its log: later requests append to the same file. It uses the model and effort chosen now, not the ones it ran with, and its history is converted as for a model switch. Only conversations whose logs are in the default directory are listed, and logs written before v0.3.0 hold no messages, so they can't be carried on. `/resume` waits until no request is running.

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
| `/resume [search]` | Carry on an earlier conversation; see [above](#carrying-on-a-conversation) |

Your models appear first; other models are grouped by provider. Each row shows catalogue pricing, context size and supported efforts. Search words match model IDs and names.

Removing the active or default model promotes the most recently picked remaining model. The last saved model cannot be removed. When the TUI has catalogue information showing a model is unavailable, it tries a remaining available model and reports the change. Headless runs do not load or refresh that catalogue.

`auto` lets Jev choose **reasoning effort for the selected model** per request. Girdle does not automatically choose a different model for each step. A pinned effort is fitted to the model's supported choices when that information is available: the requested choice, otherwise the lowest above it, otherwise the highest below it. `none` is used only when requested. A known model without an effort setting receives no effort parameter.

For headless runs, `-reasoning` sets the fallback effort. Add `-no-route` to pin it. `-reasoning` alone keeps automatic routing enabled. Headless runs send the requested effort without reading the TUI's catalogue, so it must suit the model.

## Headless runs

```bash
girdle -C /path/to/project -p "fix the failing test" -timeout 10m
girdle -C /path/to/project -p "explain the parser" -json > events.jsonl
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
| `messages` | The messages each request added to the LLM's history, in full, for carrying the conversation on |

Streaming `text_delta` events, and the `tool_start` events that mark the LLM beginning to write a tool call, reach the TUI but are excluded from both JSON output and the persistent log. `messages` events are the reverse: they are only written to the log. Concurrent event writes are serialized within the session.

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
