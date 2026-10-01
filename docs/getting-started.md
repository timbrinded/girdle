# Getting started

Girdle runs tools in the project you choose. Start in a Git checkout where you can inspect and revert its edits, and use code suitable for the selected providers' data policies.

## Requirements

- Go **1.27.0 or newer**, as declared in [go.mod](../go.mod).
- Linux or macOS, with Bash on PATH. Shell execution uses Unix process groups; native Windows execution is not supported by the current implementation.
- An [OpenRouter API key](https://openrouter.ai/settings/keys) for the LLM and a TypeSafe key for [Jev](https://docs.typesafe.ai), under the default configuration.
- [ast-grep](https://ast-grep.github.io/guide/quick-start), recommended for structural lookup and the tripwire's code checks. Its full command name is `ast-grep`; Linux's `sg` may be a different program.

For example, install ast-grep with one of its supported package managers:

```bash
brew install ast-grep                 # Homebrew
cargo install ast-grep --locked       # Cargo
ast-grep --version
```

Without ast-grep, the tripwire asks Jev about every shell command. Structural lookup falls back where possible. The [security guide](../SECURITY.md) explains the effect of that fallback.

## Install

Build from a checkout:

```bash
git clone https://github.com/timbrinded/girdle.git
cd girdle
go build -o bin/girdle ./cmd/girdle
bin/girdle -h
```

To install the checkout's binary on your PATH:

```bash
go install ./cmd/girdle
```

Go installs into `GOBIN` when set, otherwise into the `bin` directory of `GOPATH` (usually `~/go/bin`). Add that directory to your PATH if `girdle` is not found. The examples below use the checkout's `bin/girdle`.

## Configure the keys

Export the keys in the shell that starts Girdle:

```bash
export OPENROUTER_API_KEY='your-openrouter-key'
export TYPESAFE_API_KEY='your-typesafe-key'
```

Girdle reads environment variables directly. It does not load `.env` files or shell profiles itself. Keep real keys out of Git and shared logs. [OpenCode Zen](https://opencode.ai/docs/zen/) is an alternative provider for the LLM or Jev; see [configuration](configuration.md#providers).

## Run a request

```bash
bin/girdle -C /path/to/project
```

Type a request and press Enter. The TUI displays tool calls, checkpoint decisions and the final outcome. Ctrl+C cancels an active request; when idle, it exits the TUI. You can send another request in the same session after the current one ends.

For one headless request:

```bash
bin/girdle -C /path/to/project -p "fix the failing test"
```

The fallback model is `stealth/space-bunny-alpha`; saved model choices may take precedence. To use a specific model, supply its provider ID:

```bash
bin/girdle -C /path/to/project -model 'provider/model-id' -p "fix the failing test"
```

The ID above is a placeholder: choose a tool-calling model available to your account. In the OpenRouter TUI, Ctrl+L opens the searchable picker.

Add `-fast` to enable the fast flow. Inspect the resulting diff and checks before relying on the outcome. Exit `0` means the harness judged the request complete; it is not an independent proof of correctness. See [usage](usage.md#headless-runs).

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Build reports a newer Go requirement | Install Go 1.27.0 or newer and check `go version`. |
| `OPENROUTER_API_KEY` or `TYPESAFE_API_KEY` is not set | Export it in the launching shell; Girdle does not source your profile. |
| Provider rejects or withdraws a model | Choose an available model with `-model` or the TUI picker. Headless runs do not fetch the model catalogue. |
| Provider rejects a reasoning effort | In the TUI, choose an effort listed for the model. For headless use, pass a supported `-reasoning` together with `-no-route`. |
| Shell commands are blocked after disabling checkpoints | The tripwire is still on. Install ast-grep to let commands without recognised effects bypass Jev; commands needing Jev are refused while it is disabled. |
| “Sandboxed commands need macOS's sandbox-exec” | `-offline-tools` and shell restrictions from `-deny-read` require macOS. Omit them for ordinary Linux use; the benchmark runners enable them automatically. |
| A request stops or nudges unexpectedly | Inspect the `run_end` and checkpoint events in its [session log](usage.md#session-logs). Include a redacted example in a bug report. |

For changes to Girdle itself, use the [contribution guide](../CONTRIBUTING.md).
