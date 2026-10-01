# Security and data handling

Girdle executes model-selected tools with the access of the account running it. Use it on projects and with provider policies that suit your data. The shell tripwire reduces particular risks; completion decisions and safety judgements are probabilistic.

## Filesystem and shell access

Ordinary sessions have the user's filesystem and network access. Direct file tools accept absolute paths and are not confined to the selected project. The tripwire guards shell commands, including apply checks and reproduction commands; it does not mediate every direct file write.

With ast-grep available, Girdle parses shell commands and recognized inline Python/JavaScript to collect deletion, push, network and secret facts. A code floor blocks recognized catastrophic cases, including destructive operations outside the project and force-pushing shared branches. Commands with no recognized risky effects can run immediately. Other commands go to Jev, whose policy considers risk and the user's authorization.

Without a successful ast-grep parse, every command goes to Jev and the structural floor is unavailable. This parser and judgement do not constitute a general sandbox. A command requiring judgement is refused when Jev is unavailable. A refused command returns control to the user.

`-tripwire=false` disables that guard. `-no-checkpoints` disables Jev but leaves the tripwire enabled: commands requiring judgement are refused, which can include every command when ast-grep is absent.

`-offline-tools` and `-deny-read` optionally restrict shell execution through macOS `sandbox-exec`. Outside macOS, shell commands with these restrictions are refused. Direct file tools enforce denied paths separately, with the working directory exempted. The LLM and Jev calls remain online. These restrictions are used by the [benchmark](docs/benchmarks.md#isolation-and-artifacts), rather than enabled for ordinary sessions.

## Provider data

The LLM receives requests, conversation history, tool output and any supplied repository snapshot. Jev independently receives checkpoint context, which can include the task, code outlines, changes, command text and tool/check output. Changing the LLM model does not remove Jev from this data flow. For `-jev typesafe`, `GIRDLE_JEV_URL` and `GIRDLE_JEV_MODEL` can override the default endpoint and decision model, changing where that checkpoint context is sent.

The configured fallback, `stealth/space-bunny-alpha`, is an anonymous model intended here for public-code experiments. Its availability and data policy may change. The earlier `meta/muse-spark-1.3-contributor` default was also recorded as allowing training on prompts in [decision 0003](docs/decisions/0003-llm-provider.md); it is not a private-code recommendation. Check the current policies of both the selected LLM route and Jev before sending private code.

Provider keys are read from environment variables and used for authentication. The saved model list does not store them. Tool execution can still expose keys or sensitive files if a command reads them; the tripwire is not a redaction mechanism.

## Local logs

Session logs can contain prompts, source code, tool arguments/results, reasoning summaries and checkpoint state. Girdle clips some fields but does not redact sensitive content. Logs are created with mode `0644`, subject to the process umask; directory creation uses `0755`. Choose an appropriately private directory and umask for sensitive sessions.

Logs default to `~/.local/state/girdle/sessions/`, respecting `XDG_STATE_HOME`. `-log` can place them elsewhere, and `-json` also emits events to stdout. Review and redact logs before sharing them. See [usage](docs/usage.md#session-logs).

## Report a vulnerability

Use [GitHub's private vulnerability reporting](https://github.com/timbrinded/girdle/security/advisories/new). Include the affected commit, platform, relevant flags, a minimal reproduction and the impact. Use test credentials in a reproduction and omit real secrets. Ordinary defects can be reported in [issues](https://github.com/timbrinded/girdle/issues).

Girdle is developed on `master` and has no maintained release branches. Security fixes target the current branch; no response-time commitment is published.
