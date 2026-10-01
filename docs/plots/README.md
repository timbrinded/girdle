# Benchmark plots

The README figures visualize historical benchmark summaries. The CSVs transcribe their aggregate measurements; the original per-run corpus is not bundled. Regenerating the figures redraws those measurements without rerunning experiments or recomputing statistics.

| Input | Figure | Recorded source |
| --- | --- | --- |
| [warm-flow.csv](warm-flow.csv) | [Warm fast/default comparison](../assets/benchmark-warm-flow.svg) | [Optimisation stage summary](../../research/06-optimisation-stage-summary.md#headline), 26–27 September 2026 |
| [grep-context.csv](grep-context.csv) | [Grep-context comparison](../assets/benchmark-grep-context.svg) | [Decision 0020](../decisions/0020-round-trips.md#grep-context-kept-part-of--fast), reported 28 September 2026 |

The warm comparison uses Muse Spark 1.3 Contributor in both flows. Bars start at zero and show recorded mean completion times. Every run in these rows passed; labels retain each flow's passes and total runs. No uncertainty intervals were recorded in the source summary. Cold results differ and are described in that summary.

The grep-context comparison uses Space Bunny and compares grep context with the existing fast flow. Its ratios use passing runs only and a geometric mean over tasks; values below 1 favor grep context. The recorded 95% intervals bootstrap tasks, rather than individual runs, and do not capture every source of provider variation. Development pools three rounds of 33 runs per arm, while held-out tasks have 44 runs per arm. Pass counts include failures and appear as grep context versus baseline. The development improvement was not confirmed on held-out tasks.

The CSV `source` fields resolve from the repository root. Dates describe the historical experiment window or report date, as named in each CSV; they are not the plot generation date. See the [benchmark guide](../benchmarks.md#interpret-the-comparison) for the comparison method and limitations.

## Regenerate

Use Python 3.11 or newer and [uv](https://docs.astral.sh/uv/getting-started/installation/). From the repository root:

```bash
uv run --locked --script docs/plots/plot_benchmarks.py
```

The script pins Matplotlib in its inline dependency metadata; the adjacent lock file fixes its dependency resolution. It reads the CSVs with Python's standard library and overwrites the two SVGs in `docs/assets/`. Plot generation works on Linux and macOS and does not call providers or need API keys.

The plots use the project's palette and Matplotlib's bundled DejaVu Sans font, converted to vector paths. Hatched versus solid bars, open versus filled points, and direct labels preserve distinctions without color. SVG titles and descriptions provide text equivalents. Export timestamps are omitted and SVG IDs use a fixed hash salt.

To regenerate elsewhere for comparison:

```bash
uv run --locked --script docs/plots/plot_benchmarks.py --output-dir /tmp/girdle-plots
cmp docs/assets/benchmark-warm-flow.svg /tmp/girdle-plots/benchmark-warm-flow.svg
cmp docs/assets/benchmark-grep-context.svg /tmp/girdle-plots/benchmark-grep-context.svg
```

When changing measurements, update the CSVs and their source references together, then regenerate and inspect both figures for label overlap and accurate values. To change the plotting dependencies, edit the inline metadata and run `uv lock --script docs/plots/plot_benchmarks.py` before regenerating.
