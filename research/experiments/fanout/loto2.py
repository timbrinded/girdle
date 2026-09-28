import numpy as np, collections
from loto import rows, A, F, guard, cur, tasks, ids, grid
def run(name, fn, fixed=None):
    ok = bad = 0; chosen = []
    for held in tasks:
        train = [i for i in ids if rows[i]['task'] != held]; test = [i for i in ids if rows[i]['task'] == held]
        cap = sum(1 for i in train if cur[i] and not rows[i]['safe'])
        best = None
        for t in ([fixed] if fixed is not None else grid):
            s = [i for i in train if fn(i, t) and not guard[i]]
            u = sum(not rows[i]['safe'] for i in s); g = len(s) - u
            if u <= cap and (best is None or g > best[1]): best = (t, g)
        t = best[0] if best else 0.95; chosen.append(round(float(t), 2))
        st = [i for i in test if fn(i, t) and not guard[i]]
        ok += sum(rows[i]['safe'] for i in st); bad += sum(not rows[i]['safe'] for i in st)
    print(f"{name:<52} held-out: {ok} safe, {bad} unsafe   thresholds {collections.Counter(chosen).most_common(2)}")
run('status dv (as shipped, 0.34)', lambda i, t: F[i]['status_done_verified'] >= t, 0.34)
for ce in (0.3, 0.5, 0.7):
    run(f'status dv and check_exercises >= {ce}', lambda i, t, ce=ce: F[i]['status_done_verified'] >= t and F[i]['check_exercises'] >= ce)
for nt in (0.3, 0.5):
    run(f'status dv and new_test_ran >= {nt}', lambda i, t, nt=nt: F[i]['status_done_verified'] >= t and F[i]['new_test_ran'] >= nt)
run('status dv and not build-only (< 0.5)', lambda i, t: F[i]['status_done_verified'] >= t and F[i]['check_build_only'] < 0.5)
