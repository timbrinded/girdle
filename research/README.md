# Research and measurements

These are dated design explorations and experimental reports from September 2026. They preserve the inputs, observations and conclusions of those sessions. Early recommendations and measurements may predate the current implementation; start with [current docs](../docs/README.md) when using Girdle.

| Note | Scope |
| --- | --- |
| [01 — Harness and Jev](01-harness-and-jev.md) | Initial literature, architecture options and local/hosted decision-engine probes |
| [02 — Segment 1 results](02-segment-1-results.md) | First live gate and small-fixture comparisons |
| [03 — Fast-flow results](03-fast-flow-results.md) | Snapshot, batching, early-stop and race experiments |
| [04 — Scale suite](04-scale-suite.md) | Tasks and validation on larger open-source repositories |
| [05 — Fast flow at scale](05-fast-flow-at-scale.md) | Scale comparisons and performance investigation |
| [06 — Optimisation stage summary](06-optimisation-stage-summary.md) | Muse Spark stage summary, warm/cold conditions and declined ideas |

Later Space Bunny experiments are in decisions [0018](../docs/decisions/0018-space-bunny-offline-tools.md), [0019](../docs/decisions/0019-harness-time.md) and [0020](../docs/decisions/0020-round-trips.md). They introduce tighter isolation and passing-only timing, and distinguish development gains from held-out confirmation.

The [experiment scripts](experiments/README.md) retain the analysis methods. The original benchmark logs, generated replay data, warm repositories and snapshot dumps are not bundled. Running the scripts with new inputs repeats the method; it does not reproduce the original provider responses or measurements.

Use the [benchmark guide](../docs/benchmarks.md) for current runs and interpretation, and the [decision index](../docs/decisions/README.md) to trace changes. Do not treat historical prices, provider availability or anonymous model identities as current facts.
