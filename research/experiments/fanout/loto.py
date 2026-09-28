import numpy as np, collections
from analyse_step import rows, A, F, current_policy
ids = sorted(A)
for i in ids:
    F[i]['complete_rich'] = A[i]['rich'].get('complete', 0)
    reqs = rows[i]['state'].get('requirements') or []
    instr = [j for j in range(len(reqs)) if A[i]['base'].get(f'req_{j}_is_instruction', 1) >= 0.5]
    F[i]['min_req_rich'] = min([A[i]['rich'].get(f'req_{j}', 1) for j in instr] or [1])
    F[i]['combo'] = np.mean([F[i]['status_done_verified'], F[i]['next_stop'], F[i]['complete_rich'], F[i]['min_r_impl']])
guard = {}
for i in ids:
    ta = rows[i]['tests_asked'] if rows[i]['tests_asked'] is not None else A[i]['rich'].get('tests_asked', 0)
    guard[i] = ta >= 0.5 and not rows[i]['tests_changed']
cur = {i: current_policy(i) for i in ids}
tasks = sorted({rows[i]['task'] for i in ids})
pols = {
  'current questions, logged state': None,
  'current questions, rich state': lambda i, t: F[i]['complete_rich'] >= t and F[i]['min_req_rich'] >= 0.5,
  'status done_verified': lambda i, t: F[i]['status_done_verified'] >= t,
  'status dv and reqs (rich)': lambda i, t: F[i]['status_done_verified'] >= t and F[i]['min_req_rich'] >= 0.5,
  'mean of four': lambda i, t: F[i]['combo'] >= t,
}
grid = np.arange(0.2, 0.96, 0.02)
for name, fn in pols.items():
    ok = bad = 0; chosen = []
    for held in tasks:
        train = [i for i in ids if rows[i]['task'] != held]; test = [i for i in ids if rows[i]['task'] == held]
        if fn is None:
            st = [i for i in test if cur[i]]
        else:
            cap = sum(1 for i in train if cur[i] and not rows[i]['safe'])
            best = None
            for t in grid:
                s = [i for i in train if fn(i, t) and not guard[i]]
                u = sum(not rows[i]['safe'] for i in s); g = len(s) - u
                if u <= cap and (best is None or g > best[1]): best = (t, g)
            t = best[0] if best else 0.95; chosen.append(round(t, 2))
            st = [i for i in test if fn(i, t) and not guard[i]]
        ok += sum(rows[i]['safe'] for i in st); bad += sum(not rows[i]['safe'] for i in st)
    print(f"{name:<34} held-out: {ok} safe stops, {bad} unsafe   thresholds {collections.Counter(chosen).most_common(2)}")
