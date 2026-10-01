# Decision history

These records preserve the decision, rationale and evidence at the stated date. Read [current architecture](../architecture.md) and [configuration](../configuration.md) for the present implementation; later records can replace earlier defaults or remove experimental features.

| Record | Subject |
| --- | --- |
| [0001](0001-event-log.md) | Event log format |
| [0002](0002-jev-client-and-checkpoints.md) | Jev client and checkpoint structure |
| [0003](0003-llm-provider.md) | LLM provider and model |
| [0004](0004-scenarios-and-benchmark.md) | Scenarios and benchmark |
| [0005](0005-fast-flow.md) | Fast flow: fewer, larger LLM steps |
| [0006](0006-racing-and-speculative-routing.md) | Racing LLM calls and speculative routing |
| [0007](0007-fast-flow-at-scale.md) | Fast flow at scale: named-code snapshot, code tools, hedged racing |
| [0008](0008-lookup-and-three-moves.md) | One lookup, three moves, and a regression test with every fix |
| [0009](0009-cross-check.md) | An independent cross-check of each request |
| [0010](0010-speed-ideas-explored.md) | Speed ideas explored after 0009: what was kept and what wasn't |
| [0011](0011-reproduce-compaction-warm-bench.md) | Regression-test proof, compaction, warm benchmarks, and reasoning for Jev |
| [0012](0012-prune-and-test-new-ideas-only.md) | Prune what didn't pay, and test each new idea on its own |
| [0013](0013-jev-fan-out.md) | Jev fan-out: ask far more questions, keep the ones that pay |
| [0014](0014-ast-grep-with-jev.md) | ast-grep with Jev: structural facts, judged meaning |
| [0015](0015-hard-suite.md) | A hard suite of real fixes, SWE-bench style |
| [0016](0016-no-benchmark-fitting.md) | Don't fit the benchmark: a held-out set, a cleaned prompt, honest reporting |
| [0017](0017-opencode-zen.md) | OpenCode Zen: its free Jev, not its free models |
| [0018](0018-space-bunny-offline-tools.md) | Space Bunny Alpha for benchmarks, offline tool commands, and a tripwire fix |
| [0019](0019-harness-time.md) | Optimising the fast flow's time on Space Bunny: where it goes, what was tried |
| [0020](0020-round-trips.md) | Fewer round trips: grep results that show their definitions |
| [0021](0021-default-model.md) | Space Bunny Alpha is Girdle's default model |
| [0022](0022-kernel-structure.md) | One place for feature dependencies, Jev calls and turn halts |
| [0023](0023-model-picker.md) | Model picker and reasoning effort |

The original event-log and provider choices are in 0001–0003. The benchmark evolved through 0004–0020, including removed ideas and revised scoring. Records 0021–0023 establish the configured model, kernel structure and model-picker behavior. Preserve measurement dates and methods when citing them.
