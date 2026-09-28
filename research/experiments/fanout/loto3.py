import numpy as np, collections
import loto
from loto import rows, F, guard, cur, tasks, ids
grid = np.arange(0.05, 0.96, 0.03)
def run(name, fn):
    ok = bad = 0; chosen = []
    for held in tasks:
        train = [i for i in ids if rows[i]['task'] != held]; test = [i for i in ids if rows[i]['task'] == held]
        cap = sum(1 for i in train if cur[i] and not rows[i]['safe'])
        best = None
        for t in grid:
            s = [i for i in train if fn(i, t) and not guard[i]]
            u = sum(not rows[i]['safe'] for i in s); g = len(s) - u
            if u <= cap and (best is None or g > best[1]): best = (t, g)
        t = best[0]; chosen.append(round(float(t), 2))
        st = [i for i in test if fn(i, t) and not guard[i]]
        ok += sum(rows[i]['safe'] for i in st); bad += sum(not rows[i]['safe'] for i in st)
    print(f"{name:<44} held-out: {ok} safe, {bad} unsafe   status thresholds {collections.Counter(chosen).most_common(3)}")
    return ok, bad
for ce in (0.5, 0.6, 0.65, 0.7, 0.75, 0.8):
    run(f'status dv >= t and check_exercises >= {ce}', lambda i, t, ce=ce: F[i]['status_done_verified'] >= t and F[i]['check_exercises'] >= ce)
# fixed pair, all data
for t, ce in ((0.34, 0.7), (0.2, 0.7), (0.34, 0.6)):
    s = [i for i in ids if F[i]['status_done_verified'] >= t and F[i]['check_exercises'] >= ce and not guard[i]]
    print(f"fixed status {t}, exercises {ce}: {sum(rows[i]['safe'] for i in s)} safe, {sum(not rows[i]['safe'] for i in s)} unsafe (all data)")
