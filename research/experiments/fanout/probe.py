from jevlib import REPO
import json, glob, statistics as st
from jevlib import ask, noul
f = sorted(glob.glob(REPO + '/bench/results/*/*/events.jsonl'))[-50]
state = None
for l in open(f):
    e = json.loads(l)
    if e.get('type') == 'decision': state = e['decision']['state']
state = state or {"task": "x"}
state["pad"] = ["filler line %d about nothing in particular" % i for i in range(150)]
for n in (1, 10, 40, 100, 200):
    qs = {f"q{i}": noul(f"Does `task` mention the number {i}?") for i in range(n)}
    ms = []
    for _ in range(3):
        r = ask(state, qs); ms.append(r["_ms"])
    print(n, "questions:", "median", st.median(ms), "ms", "input tokens", r["usage"]["input_tokens"], "answers", len(r["answers"]))
