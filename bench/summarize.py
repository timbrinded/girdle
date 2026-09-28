"""Summarise a benchmark results directory: pass rate, time and cost per agent."""

import json
import sys
from collections import defaultdict
from pathlib import Path

root = Path(sys.argv[1])
rows = [json.loads(p.read_text()) for p in sorted(root.glob("*/result.json"))]
invalid = [r for r in rows if r.get("invalid")]
rows = [r for r in rows if not r.get("invalid")]
if invalid:
    print(f"excluded {len(invalid)} invalid run(s): " + ", ".join(f"{r['task']}.{r['agent']}.{r['rep']}" for r in invalid))
if not rows:
    sys.exit(f"no results in {root}")

agents = sorted({r["agent"] for r in rows}, key=lambda a: (a != "girdle", a))
tasks = sorted({r["task"] for r in rows})
by = defaultdict(list)
for r in rows:
    by[(r["task"], r["agent"])].append(r)


def cell(rs):
    if not rs:
        return "-"
    return f"{sum(r['pass'] for r in rs)}/{len(rs)}"


width = 15
print(f"\n{'task':<16}" + "".join(f"{a:>{width}}" for a in agents))
for t in tasks:
    print(f"{t:<16}" + "".join(f"{cell(by[(t, a)]):>{width}}" for a in agents))

print()
summary = {}
for a in agents:
    rs = [r for r in rows if r["agent"] == a]
    n = len(rs)
    passed = sum(r["pass"] for r in rs)
    summary[a] = {
        "runs": n,
        "passed": passed,
        "pass_rate": round(passed / n, 3),
        "mean_secs": round(sum(r["secs"] for r in rs) / n, 1),
        "mean_cost_usd": round(sum(r["cost_usd"] for r in rs) / n, 5),
        "mean_tool_calls": round(sum(r["tool_calls"] for r in rs) / n, 1),
        "mean_nudges": round(sum(r.get("nudges", 0) for r in rs) / n, 2),
        "timeouts": sum(r["agent_exit"] == 124 for r in rs),
    }
    s = summary[a]
    print(f"{a:<14} pass {passed}/{n} ({s['pass_rate']:.0%})  mean {s['mean_secs']}s  ${s['mean_cost_usd']:.4f}/run  "
          f"{s['mean_tool_calls']} tool calls  {s['mean_nudges']} nudges  {s['timeouts']} timeouts")
(root / "summary.json").write_text(json.dumps(summary, indent=2))
