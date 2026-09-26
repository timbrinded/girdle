"""Write result.json for one benchmark run from its logs."""

import json
import sys
from pathlib import Path

out, task, agent, rep, secs, agent_exit, check_exit = sys.argv[1:8]
out = Path(out)


def lines(path):
    if not path.exists():
        return []
    rows = []
    for line in path.read_text(errors="replace").splitlines():
        line = line.strip()
        if line.startswith("{"):
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError:
                pass
    return rows


result = {
    "task": task,
    "agent": agent,
    "rep": int(rep),
    "pass": int(check_exit) == 0,
    "secs": int(secs),
    "agent_exit": int(agent_exit),
    "tool_calls": 0,
    "input_tokens": 0,
    "cache_read_tokens": 0,
    "output_tokens": 0,
    "nudges": 0,
    "decisions": 0,
    "outcome": "",
    "reason": "",
}

if agent.startswith("girdle"):
    for e in lines(out / "events.jsonl"):
        t = e.get("type")
        if t == "tool_call":
            result["tool_calls"] += 1
        elif t == "nudge":
            result["nudges"] += 1
        elif t == "decision":
            result["decisions"] += 1
        elif t == "run_end":
            u = e.get("usage") or {}
            result["input_tokens"] = u.get("input_tokens", 0)
            result["cache_read_tokens"] = u.get("cache_read_tokens", 0)
            result["output_tokens"] = u.get("output_tokens", 0)
            result["jev_tokens"] = u.get("jev_tokens", 0)
            result["fast_input_tokens"] = u.get("fast_input_tokens", 0)
            result["fast_cache_read_tokens"] = u.get("fast_cache_read_tokens", 0)
            result["fast_output_tokens"] = u.get("fast_output_tokens", 0)
            result["outcome"] = e.get("outcome", "")
            result["reason"] = e.get("reason", "")
else:
    for e in lines(out / "pi.jsonl"):
        t = e.get("type")
        if t == "tool_execution_start":
            result["tool_calls"] += 1
        elif t == "message_end":
            m = e.get("message") or {}
            if m.get("role") != "assistant":
                continue
            u = m.get("usage") or {}
            result["input_tokens"] += u.get("input", 0)
            result["cache_read_tokens"] += u.get("cacheRead", 0)
            result["output_tokens"] += u.get("output", 0)
            result["outcome"] = m.get("stopReason", result["outcome"])

# A check that fails only because the agent's own test helpers collide with
# names in our hidden tests says nothing about the agent: mark it invalid.
check_text = (out / "check.txt").read_text(errors="replace") if (out / "check.txt").exists() else ""
result["invalid"] = (not result["pass"]) and "redeclared in this block" in check_text and "zz_hidden" in check_text

# Both agents report input excluding cache reads. Muse Spark 1.3 Contributor
# on OpenRouter: $0.10/M input, $0.002/M cache reads, $0.20/M output. The
# fast model's share (gpt-oss-120b on Groq, $0.15/M input, $0.60/M output)
# is priced at its own rates, cache reads at the full input rate so as not
# to flatter it. Jev: $0.042/M input.
fast_in = result.get("fast_input_tokens", 0)
fast_cache = result.get("fast_cache_read_tokens", 0)
fast_out = result.get("fast_output_tokens", 0)
result["cost_usd"] = round(
    (result["input_tokens"] - fast_in) * 0.10e-6
    + (result["cache_read_tokens"] - fast_cache) * 0.002e-6
    + (result["output_tokens"] - fast_out) * 0.20e-6
    + (fast_in + fast_cache) * 0.15e-6
    + fast_out * 0.60e-6
    + result.get("jev_tokens", 0) * 0.042e-6,
    5,
)
(out / "result.json").write_text(json.dumps(result, indent=2))
mark = "PASS" if result["pass"] else "fail"
print(f"{mark}  {task:<14} {agent:<13} rep{rep}  {secs:>4}s  tools={result['tool_calls']:<3} nudges={result['nudges']}  ${result['cost_usd']:.4f}  {result['outcome']} {result['reason']}")
