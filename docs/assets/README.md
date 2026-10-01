# SVG assets

| Asset | Purpose | Size |
| --- | --- | --- |
| [girdle-logo.svg](girdle-logo.svg) | Geometric G with a belt and buckle | 256 × 256 |
| [girdle-banner.svg](girdle-banner.svg) | README banner and project introduction | 1200 × 320 |
| [architecture.svg](architecture.svg) | Current component ownership and request outcomes | 1200 × 650 |
| [benchmark-warm-flow.svg](benchmark-warm-flow.svg) | Warm fast/default completion times and pass counts | 720 × 432 pt |
| [benchmark-grep-context.svg](benchmark-grep-context.svg) | Development and held-out time ratios with 95% intervals | 720 × 417.6 pt |

The SVGs are self-contained, with no scripts or external images, styles or fonts. Each has an accessible title and description. The banner and diagram use a system sans-serif stack; the logo contains only vector shapes. The benchmark plots use Matplotlib with text converted to paths; their [CSV inputs and generation instructions](../plots/README.md) preserve the recorded evidence. Keep the diagram aligned with [architecture](../architecture.md) when behavior changes.

Palette: warm paper `#f5f2e9`, ink `#202822`, green `#285c48` and orange `#bd5637`. Original assets are licensed under the repository's [Apache-2.0 license](../../LICENSE).
