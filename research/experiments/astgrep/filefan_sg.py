"""Prefetch recall with ast-grep outlines instead of regex outlines."""
from jevlib import REPO
import json, sys, concurrent.futures as cf, collections
from jevlib import ask
sys.path.insert(0, '../fanout')
from filefan import TASKS, G, files, in_snapshot, Q
from units import units
d = json.load(open('filefan.json'))
U = {}
for t in TASKS:
    by = collections.defaultdict(list)
    for u in units(G + f'bench/.warm/girdle-fast/{t}/repo', 'go' if t.startswith('gm') else 'python'):
        by[u['file']].append(u)
    U[t] = by
def sg_outline(task, p):
    try: lines = open(G + f'bench/.warm/girdle-fast/{task}/repo/{p}', errors='replace').read().splitlines()
    except Exception: return ''
    out = lines[:10] + ['…']
    for u in sorted(U[task].get(p, []), key=lambda u: u['start'])[:80]:
        doc = lines[u['start'] - 2].strip() if u['start'] >= 2 and lines[u['start'] - 2].strip().startswith(('//', '#')) else ''
        if u['head'].startswith(('import', 'from ', '"""', "'''")): continue
        out.append((doc + '\n' if doc else '') + u['head'])
    return '\n'.join(out)[:6000]
res = {}
with cf.ThreadPoolExecutor(16) as ex:
    jobs = []
    for task in TASKS:
        prompt = open(G + f'bench/scale/{task}/task.md').read().strip()
        for p in files(task):
            if 'html5entities' in p: continue
            jobs.append((task, p, ex.submit(ask, {'task': prompt, 'file': {'path': p, 'outline': sg_outline(task, p)}}, Q)))
    for task, p, f in jobs:
        try: res.setdefault(task, {})[p] = f.result()['answers']['need_read']['noul']
        except Exception as e: print('error', p, str(e)[:80], file=sys.stderr)
looked, nruns = d['looked'], d['nruns']
tot = {'regex outline': [0, 0], 'ast-grep outline': [0, 0]}; W = 0
for task in TASKS:
    snap = in_snapshot(task); cands = [p for p in res[task] if p not in snap]
    target = {p for p, c in looked[task].items() if c / nruns[task] >= 0.25 and p in cands}
    W += len(target)
    for name, sc in (('regex outline', {p: d['res'][task].get(p, {}).get('need_read', 0) for p in cands}), ('ast-grep outline', res[task])):
        order = sorted(cands, key=lambda p: -sc.get(p, 0))
        tot[name][0] += len(target & set(order[:5])); tot[name][1] += len(target & set(order[:10]))
print({k: f"{v[0]}@5 {v[1]}@10 of {W}" for k, v in tot.items()})
