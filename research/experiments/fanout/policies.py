from jevlib import REPO
import json, collections, datetime, numpy as np
from analyse_step import rows, A, F, current_policy
ROOT = REPO + '/bench/results/'
cache = {}
def left(i):
    """Seconds from decision i to the end of its run."""
    r = rows[i]
    if r['run'] not in cache:
        cache[r['run']] = [json.loads(l) for l in open(ROOT + r['run'] + '/events.jsonl')]
    E = cache[r['run']]
    t = lambda e: datetime.datetime.fromisoformat(e['time'].replace('Z', '+00:00')).timestamp()
    ds = [e for e in E if e.get('type') == 'decision' and e['decision'].get('checkpoint') == 'step_end']
    k = sum(1 for j in range(i) if rows[j]['run'] == r['run'])
    return t(E[-1]) - t(ds[k]) if k < len(ds) else 0
ids = sorted(A)
safe = {i: rows[i]['safe'] for i in ids}
cur = {i: current_policy(i) for i in ids}
guard = {}
for i in ids:
    ta = rows[i]['tests_asked'] if rows[i]['tests_asked'] is not None else A[i]['rich'].get('tests_asked', 0)
    guard[i] = ta >= 0.5 and not rows[i]['tests_changed']
sc = [left(i) for i in ids if not cur[i] and safe[i]]
print(f"a safe continue costs a median {np.median(sc):.1f} s, mean {np.mean(sc):.1f} s until the run ends ({len(sc)} cases)")

def evaluate(name, stopfn):
    st = {i for i in ids if stopfn(i) and not guard[i]}
    s_ok = sum(safe[i] for i in st); s_bad = len(st) - s_ok
    saved = sum(left(i) for i in st if not cur[i] and safe[i])
    return name, s_ok, s_bad, saved
cur_ok = sum(safe[i] for i in ids if cur[i]); cur_bad = sum(1 for i in ids if cur[i] and not safe[i])
print(f"current: {cur_ok} safe stops, {cur_bad} unsafe")
def feat(i, k): return F[i].get(k, 0)
for i in ids:
    F[i]['complete_rich'] = A[i]['rich'].get('complete', 0)
    reqs = rows[i]['state'].get('requirements') or []
    instr = [j for j in range(len(reqs)) if A[i]['base'].get(f'req_{j}_is_instruction', 1) >= 0.5]
    F[i]['min_req_rich'] = min([A[i]['rich'].get(f'req_{j}', 1) for j in instr] or [1])
    F[i]['remaining_n'] = 1 - (F[i]['remaining'] - 1) / 3 if F[i]['remaining'] >= 1 else 1 - F[i]['remaining'] / 3
    F[i]['combo'] = np.mean([F[i]['status_done_verified'], F[i]['next_stop'], F[i]['complete_rich'], F[i]['min_r_impl']])
cands = {
    'same questions, rich state': lambda i, t: feat(i, 'complete_rich') >= t and feat(i, 'min_req_rich') >= 0.5,
    'status done_verified': lambda i, t: feat(i, 'status_done_verified') >= t,
    'next = stop': lambda i, t: feat(i, 'next_stop') >= t,
    'min requirement implemented': lambda i, t: feat(i, 'min_r_impl') >= t,
    'complete_rich and min impl': lambda i, t: feat(i, 'complete_rich') >= t and feat(i, 'min_r_impl') >= 0.5,
    'mean of four': lambda i, t: feat(i, 'combo') >= t,
}
print(f"\n{'policy':<30} {'thr':>5} {'safe':>6} {'unsafe':>7} {'s saved':>8}")
best = {}
for name, fn in cands.items():
    pts = []
    for t in np.arange(0.30, 0.99, 0.02):
        pts.append((t,) + evaluate(name, lambda i: fn(i, t))[1:])
    # best points: most safe stops with unsafe <= current; fewest unsafe with safe >= current
    a = max([p for p in pts if p[2] <= cur_bad] or [pts[-1]], key=lambda p: (p[1], -p[2]))
    b = min([p for p in pts if p[1] >= cur_ok] or [pts[0]], key=lambda p: (p[2], -p[1]))
    for tag, p in (('same unsafe', a), ('same safe', b)):
        print(f"{name:<30} {p[0]:5.2f} {p[1]:6d} {p[2]:7d} {p[3]:8.0f}   [{tag}]")
