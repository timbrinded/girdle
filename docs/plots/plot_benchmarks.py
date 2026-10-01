# /// script
# requires-python = ">=3.11"
# dependencies = ["matplotlib==3.11.2"]
# ///
"""Render the README's historical benchmark summaries as deterministic SVGs."""

import argparse
import csv
import math
import xml.etree.ElementTree as ET
from datetime import date
from pathlib import Path

import matplotlib

matplotlib.use("Agg")

import matplotlib.pyplot as plt
from matplotlib.axes import Axes
from matplotlib.figure import Figure
from matplotlib.patches import Patch

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
PAPER = "#f5f2e9"
INK = "#202822"
GREEN = "#285c48"
ORANGE = "#bd5637"
GRID = "#d8ded7"
SVG = "http://www.w3.org/2000/svg"


def read_rows(filename: str) -> list[dict[str, str]]:
    with (HERE / filename).open(newline="", encoding="utf-8") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        raise ValueError(f"{filename} has no measurements")
    for row in rows:
        source = ROOT / row["source"].split("#", 1)[0]
        if not source.is_file():
            raise ValueError(f"Missing measurement source: {source}")
    return rows


def common(rows: list[dict[str, str]], field: str) -> str:
    values = {row[field] for row in rows}
    if len(values) != 1:
        raise ValueError(f"Figure requires a shared {field}: {values}")
    return values.pop()


def period(start: str, end: str) -> str:
    first, last = date.fromisoformat(start), date.fromisoformat(end)
    if first == last:
        return f"{first.day} {first:%B %Y}"
    if (first.year, first.month) == (last.year, last.month):
        return f"{first.day}–{last.day} {last:%B %Y}"
    return f"{first:%d %B %Y} – {last:%d %B %Y}"


def style_axes(ax: Axes) -> None:
    ax.spines[["top", "right"]].set_visible(False)
    ax.tick_params(axis="y", length=0, pad=12)
    ax.grid(axis="x", color=GRID, linewidth=0.7)
    ax.set_axisbelow(True)


def save_svg(fig: Figure, output: Path, title: str, description: str) -> None:
    fig.savefig(output, metadata={"Date": None, "Title": title})
    plt.close(fig)
    # Matplotlib paths keep the font self-contained. Add an accessible text
    # equivalent to the SVG root as well as the README image's alt text.
    parser = ET.XMLParser(target=ET.TreeBuilder(insert_comments=True))
    tree = ET.parse(output, parser=parser)
    root = tree.getroot()
    root.set("role", "img")
    root.set("aria-labelledby", "chart-title chart-description")
    root.insert(0, ET.Element(f"{{{SVG}}}title", id="chart-title"))
    root.insert(1, ET.Element(f"{{{SVG}}}desc", id="chart-description"))
    root[0].text = title
    root[1].text = description
    ET.register_namespace("", SVG)
    ET.register_namespace("xlink", "http://www.w3.org/1999/xlink")
    tree.write(output, encoding="utf-8", xml_declaration=True)
    print(output)


def warm_flow(output_dir: Path) -> None:
    rows = read_rows("warm-flow.csv")
    title = "Fast flow versus default"
    model = common(rows, "model")
    conditions = common(rows, "conditions")
    dates = period(common(rows, "date_start"), common(rows, "date_end"))
    fig, ax = plt.subplots(figsize=(10, 6))
    fig.subplots_adjust(left=0.25, right=0.97, top=0.73, bottom=0.25)
    fig.text(0.04, 0.93, title, fontsize=20, weight="bold")
    fig.text(0.04, 0.85, f"{model} · {conditions.title()} runs · {dates}", fontsize=13)

    maximum = max(
        float(row[f"{arm}_mean_seconds"])
        for row in rows
        for arm in ("baseline", "candidate")
    )
    ax.set_xlim(0, math.ceil(maximum / 10) * 15)
    positions = list(range(len(rows)))
    descriptions = []
    for position, row in zip(positions, rows):
        descriptions.append(f"{row['label']}, {row['tasks']} tasks")
        for arm, offset, color, hatch in (
            ("baseline", -0.17, ORANGE, "//"),
            ("candidate", 0.17, GREEN, None),
        ):
            seconds = float(row[f"{arm}_mean_seconds"])
            passed, runs = row[f"{arm}_passed"], row[f"{arm}_runs"]
            ax.barh(
                position + offset,
                seconds,
                height=0.27,
                color=color,
                edgecolor=INK,
                linewidth=0.7,
                hatch=hatch,
            )
            ax.text(
                seconds + maximum * 0.025,
                position + offset,
                f"{seconds:.1f} s · {passed}/{runs}",
                va="center",
                fontsize=12,
            )
            descriptions.append(
                f"{row[arm]}: mean {seconds:.1f} seconds, {passed} of {runs} passed"
            )
    ax.set_yticks(positions, [f"{row['label']}\n{row['tasks']} tasks" for row in rows])
    ax.set_ylim(len(rows) - 0.45, -0.55)
    ax.set_xlabel("Mean completion time (seconds) · lower is better", labelpad=12)
    ax.legend(
        handles=[
            Patch(
                facecolor=ORANGE,
                edgecolor=INK,
                hatch="//",
                label=common(rows, "baseline"),
            ),
            Patch(facecolor=GREEN, edgecolor=INK, label=common(rows, "candidate")),
        ],
        loc="lower left",
        bbox_to_anchor=(0, 1.02),
        ncol=2,
        frameon=False,
        borderaxespad=0,
    )
    style_axes(ax)
    fig.text(
        0.04,
        0.11,
        "Bar labels: mean seconds · passes/runs. No uncertainty intervals recorded.",
        fontsize=11,
    )
    fig.text(
        0.04,
        0.055,
        "Warm runs only; cold comparisons differ. Source: optimisation stage summary.",
        fontsize=11,
    )
    save_svg(
        fig,
        output_dir / "benchmark-warm-flow.svg",
        title,
        f"{model}, {conditions} runs, {dates}. "
        + ". ".join(descriptions)
        + ". Means only; uncertainty intervals were not recorded. Cold results differ. "
        + f"Source: {common(rows, 'source')}.",
    )


def grep_context(output_dir: Path) -> None:
    rows = read_rows("grep-context.csv")
    title = "Grep context across task sets"
    model = common(rows, "model")
    report_date = common(rows, "report_date")
    level = common(rows, "ci_level")
    fig, ax = plt.subplots(figsize=(10, 5.8))
    fig.subplots_adjust(left=0.25, right=0.66, top=0.72, bottom=0.26)
    fig.text(0.04, 0.93, title, fontsize=20, weight="bold")
    fig.text(
        0.04,
        0.85,
        f"{model} · Reported {period(report_date, report_date)}",
        fontsize=13,
    )
    low = min(float(row["ci_low"]) for row in rows)
    high = max(float(row["ci_high"]) for row in rows)
    ax.set_xlim(math.floor(low * 10) / 10, math.ceil(high * 10) / 10 + 0.1)
    ax.axvline(1, color=INK, linestyle="--", linewidth=1.2)
    ax.text(
        1,
        1.04,
        "Same speed",
        transform=ax.get_xaxis_transform(),
        ha="center",
        fontsize=11,
    )
    ax.text(
        1.12,
        1.04,
        f"Ratio [{level}% interval]\nPasses: context vs baseline",
        transform=ax.transAxes,
        fontsize=11,
    )
    descriptions = []
    positions = list(range(len(rows)))
    for position, row in zip(positions, rows):
        point, lower, upper = (
            float(row[key]) for key in ("time_ratio", "ci_low", "ci_high")
        )
        if not 0 < lower <= point <= upper:
            raise ValueError(f"Invalid time-ratio interval: {row}")
        ax.errorbar(
            point,
            position,
            xerr=[[point - lower], [upper - point]],
            fmt="o",
            color=GREEN,
            markerfacecolor=GREEN if position == 0 else PAPER,
            markeredgewidth=1.8,
            markersize=8,
            capsize=5,
            linewidth=1.8,
        )
        ax.text(
            1.12,
            position,
            f"{point:.2f} [{lower:.2f}–{upper:.2f}]\n"
            f"{row['candidate_passed']}/{row['candidate_runs']} vs "
            f"{row['baseline_passed']}/{row['baseline_runs']}",
            transform=ax.get_yaxis_transform(),
            va="center",
            linespacing=1.6,
            fontsize=13,
        )
        descriptions.append(
            f"{row['label']}, {row['tasks']} tasks: ratio {point:.2f}, "
            f"{level}% interval {lower:.2f} to {upper:.2f}; "
            f"{row['candidate']} passed {row['candidate_passed']}/{row['candidate_runs']}, "
            f"{row['baseline']} passed {row['baseline_passed']}/{row['baseline_runs']}"
        )
    ax.set_yticks(positions, [f"{row['label']}\n{row['tasks']} tasks" for row in rows])
    ax.set_ylim(len(rows) - 0.4, -0.6)
    ax.set_xlabel("Context / baseline time · below 1 is faster", labelpad=12)
    style_axes(ax)
    fig.text(
        0.04,
        0.12,
        f"Passing-run ratios; {level}% task-bootstrap intervals. Source: decision 0020.",
        fontsize=11,
    )
    fig.text(
        0.04,
        0.06,
        "Pass counts include failures; timing ratios use passing runs only.",
        fontsize=11,
    )
    save_svg(
        fig,
        output_dir / "benchmark-grep-context.svg",
        title,
        f"{model}, reported {period(report_date, report_date)}. "
        + ". ".join(descriptions)
        + ". Ratios compare grep context with the existing fast flow, "
        "using passing runs only and a geometric mean over tasks. "
        f"{level}% intervals bootstrap tasks; below 1 is faster. "
        + f"Source: {common(rows, 'source')}.",
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, default=HERE.parent / "assets")
    args = parser.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    plt.rcParams.update(
        {
            "font.family": "DejaVu Sans",
            "font.size": 13,
            "text.color": INK,
            "axes.labelcolor": INK,
            "axes.edgecolor": INK,
            "axes.linewidth": 0.7,
            "xtick.color": INK,
            "ytick.color": INK,
            "figure.facecolor": PAPER,
            "axes.facecolor": PAPER,
            "svg.fonttype": "path",
            "svg.hashsalt": "girdle-benchmark-plots",
        }
    )
    warm_flow(args.output_dir)
    grep_context(args.output_dir)


if __name__ == "__main__":
    main()
