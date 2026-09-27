"""Compare two agents in one benchmark results directory, paired by task.

    python3 bench/compare.py <results-dir> <baseline-agent> <candidate-agent>

Time is compared as the geometric mean, across tasks, of each task's ratio
of mean log run time (candidate over baseline). A 95% interval comes from
resampling tasks. Runs that timed out count at their time limit, and failed
runs count too: a fast failure isn't a speed-up. The verdict is:

- FASTER: the whole interval is below 1, the candidate fails at most one
  run more, and it uses at most 10% more LLM tokens.
- FASTER, BY COMPUTE: faster, but it uses more than 10% more tokens.
- SLOWER: the whole interval is above 1.
- NO CLEAR DIFFERENCE: anything else.
"""
import collections
import glob
import json
import math
import os
import random
import statistics
import sys

if len(sys.argv) != 4:
    sys.exit(__doc__)
root, base, cand = sys.argv[1:]


def load(agent):
    rows = collections.defaultdict(list)
    for f in glob.glob(os.path.join(root, f"*.{agent}.*", "result.json")):
        r = json.load(open(f))
        if r.get("agent") != agent or r.get("invalid"):
            continue
        rows[r["task"]].append(r)
    return rows


A, B = load(base), load(cand)
tasks = sorted(set(A) & set(B))
if not tasks:
    sys.exit(f"no tasks run by both {base} and {cand} in {root}")


def mean_log(rs):
    return statistics.mean(math.log(max(r["secs"], 1)) for r in rs)


ratios = {t: mean_log(B[t]) - mean_log(A[t]) for t in tasks}
point = math.exp(statistics.mean(ratios.values()))
rng = random.Random(0)
boot = sorted(
    math.exp(statistics.mean(ratios[rng.choice(tasks)] for _ in tasks)) for _ in range(10000)
)
lo, hi = boot[249], boot[9749]


def total(rows, key):
    return sum(r.get(key, 0) or 0 for t in tasks for r in rows[t])


def runs(rows):
    return [r for t in tasks for r in rows[t]]


passA, passB = total(A, "pass"), total(B, "pass")
nA, nB = len(runs(A)), len(runs(B))
tokA = sum((r.get("input_tokens", 0) + r.get("cache_read_tokens", 0) + r.get("output_tokens", 0)) for r in runs(A)) / nA
tokB = sum((r.get("input_tokens", 0) + r.get("cache_read_tokens", 0) + r.get("output_tokens", 0)) for r in runs(B)) / nB
costA = statistics.mean(r.get("cost_usd", 0) for r in runs(A))
costB = statistics.mean(r.get("cost_usd", 0) for r in runs(B))

print(f"{'task':<24}{base:>16}{cand:>16}  ratio")
for t in tasks:
    ma = statistics.mean(r["secs"] for r in A[t])
    mb = statistics.mean(r["secs"] for r in B[t])
    pa = sum(r["pass"] for r in A[t])
    pb = sum(r["pass"] for r in B[t])
    print(f"{t:<24}{f'{pa}/{len(A[t])} {ma:.0f}s':>16}{f'{pb}/{len(B[t])} {mb:.0f}s':>16}  {math.exp(ratios[t]):.2f}")
print()
print(f"time ratio (geometric mean over {len(tasks)} tasks): {point:.2f}, 95% interval {lo:.2f} to {hi:.2f}")
print(f"passed: {base} {passA}/{nA}, {cand} {passB}/{nB}")
print(f"median time: {statistics.median(r['secs'] for r in runs(A)):.0f}s against {statistics.median(r['secs'] for r in runs(B)):.0f}s")
print(f"LLM tokens per run: {tokA:.0f} against {tokB:.0f} ({tokB / tokA:.2f}x); cost per run ${costA:.4f} against ${costB:.4f}")
if hi < 1 and passB >= passA - 1:
    verdict = "FASTER" if tokB <= tokA * 1.1 else "FASTER, BY COMPUTE"
elif lo > 1:
    verdict = "SLOWER"
else:
    verdict = "NO CLEAR DIFFERENCE"
print(f"verdict: {verdict}")
