from jevlib import REPO
import json, collections
from analyse_step import rows, A, F, current_policy
ROOT = REPO + '/bench/results/'
bad = [i for i in sorted(A) if current_policy(i) and not rows[i]['safe']]
print(collections.Counter(rows[i]['agent'] for i in bad)); print(collections.Counter(rows[i]['task'] for i in bad))
for i in bad:
    r = rows[i]; x = A[i]['rich']; reqs = r['state'].get('requirements') or []
    impl = [(j, x.get(f'r{j}_impl'), x.get(f'r{j}_tested')) for j in range(len(reqs))]
    worst = min(impl, key=lambda t: t[1] if t[1] is not None else 1) if impl else None
    chk = open(ROOT + r['run'] + '/check.txt', errors='replace').read()
    fail = [l.strip() for l in chk.splitlines() if any(w in l for w in ('FAIL', 'Error', 'assert', 'expected', 'got', 'Expected'))][:3]
    print(f"\n{r['task']} {r['agent']} min_impl={F[i]['min_r_impl']:.2f} status_dv={F[i]['status_done_verified']:.2f} edge_unhandled={x.get('edge_unhandled',0):.2f}")
    if worst: print(f"  weakest req[{worst[0]}] impl={worst[1]:.2f} tested={worst[2]:.2f}: {reqs[worst[0]][:160]}")
    print("  hidden failure:", ' | '.join(fail)[:300])
