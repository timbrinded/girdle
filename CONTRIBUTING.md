# Contributing

Girdle is an experimental terminal coding agent. Contributions should improve a behavior that matters in real work and carry evidence appropriate to that change. Start with the [architecture](docs/architecture.md), [current configuration](docs/configuration.md) and [development principles](CLAUDE.md).

## Set up and verify

Use Go 1.27.0 or newer. From the repository root:

```bash
go build -o bin/girdle ./cmd/girdle
go test ./...
go vet ./...
```

The normal test suite does not need provider keys. Install ast-grep to exercise its parser-dependent checks; tests skip platform-specific sandbox cases where appropriate. Live gates and benchmarks use real providers and are described separately in the [benchmark guide](docs/benchmarks.md).

Format changed Go files with `gofmt`. Run focused checks while working, then the build, test and vet commands before submitting. For a behavioral bug, include a regression check that fails without the fix. Documentation changes need valid links, accurate examples and readable diagrams; they do not need tests that merely repeat the prose.

## Propose a change

Use a branch such as `fix/description`, `feat/description` or `docs/description`, and open a pull request against `master`. Describe the concrete problem, resulting behavior and validation. For a bug report, include the commit, platform, flags, model/effort, observed outcome and a minimal example. Share only redacted logs. Report vulnerabilities through the [security policy](SECURITY.md).

Keep changes focused. Jev questions belong with their checkpoint policies; feature prerequisites belong in `kernel.Config.resolved`. Code reads typed facts and applies policies; Jev judges meaning. Preserve the code floor under the shell tripwire and document any changed access boundary.

Record a non-obvious, durable decision under `docs/decisions/` with its date and rationale, then add it to the [index](docs/decisions/README.md). Update the user guides when flags, defaults, lifecycle or data handling change. Never add secrets to fixtures, logs or examples.

## Measure changes to the harness

Develop on `bench/tasks`, `bench/scale` and `bench/hard`. Compare a new idea against the current fast flow on tasks it can affect, using matched models and conditions. Report accuracy, passing-only timing and cost together. Keep speed from fewer round trips separate from speed bought by racing more calls.

Held-out tasks are for milestone evaluation. Do not tune prompts, thresholds or implementations from their contents or results, or encode hidden-test examples into prompts. [Decision 0016](docs/decisions/0016-no-benchmark-fitting.md) records this boundary. [Benchmarks](docs/benchmarks.md) describes the isolation limits and comparison rules.

Use dependencies already in [go.mod](go.mod) where they fit. Girdle's original code is Apache-2.0; preserve notices for any third-party material. Project policy prohibits copying code from Crush; shared permissively licensed libraries are separate dependencies.
