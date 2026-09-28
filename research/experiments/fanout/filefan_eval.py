import json, re, subprocess, collections
from filefan import TASKS, in_snapshot, repo, G
d = json.load(open('filefan.json'))
res, looked, changed, nruns = d['res'], d['looked'], d['changed'], d['nruns']
def names(task):
    t = open(G + f'bench/scale/{task}/task.md').read()
    ns = set()
    for m in re.findall(r'`([^`]+)`', t):
        for w in re.findall(r'[A-Za-z_][A-Za-z0-9_]{2,}', m): ns.add(w.split('.')[-1])
    return ns
def grep_rank(task, cands):
    ns = names(task); sc = {}
    for p in cands:
        try: txt = open(f"{repo(task)}/{p}", errors='replace').read()
        except Exception: txt = ''
        sc[p] = sum(len(re.findall(r'\b' + re.escape(n) + r'\b', txt)) for n in ns)
    return sc
print(f"{'task':<18} {'target':>6}  " + '  '.join(f"{m:>18}" for m in ('need_read', 'example', 'relevance', 'combo', 'grep names')))
tot = collections.defaultdict(lambda: [0, 0])
for task in TASKS:
    snap = in_snapshot(task)
    cands = [p for p in res[task] if p not in snap]
    n = nruns[task]
    target = {p for p, c in looked[task].items() if c / n >= 0.25 and p in cands}
    if not target: 
        print(task, 'no target'); continue
    g = grep_rank(task, cands)
    ranks = {
        'need_read': {p: res[task][p]['need_read'] for p in cands},
        'example': {p: res[task][p]['example'] for p in cands},
        'relevance': {p: res[task][p]['relevance'] for p in cands},
        'combo': {p: res[task][p]['need_read'] + res[task][p]['example'] + res[task][p]['will_change'] for p in cands},
        'grep names': g,
    }
    cells = []
    for m, sc in ranks.items():
        order = sorted(cands, key=lambda p: -sc[p])
        hits5 = len(target & set(order[:5])); hits10 = len(target & set(order[:10]))
        tot[m][0] += hits5; tot[m][1] += hits10
        cells.append(f"{hits5:>2}@5 {hits10:>2}@10 of {len(target)}")
    print(f"{task:<18} {len(target):>6}  " + '  '.join(f"{c:>18}" for c in cells))
    print('   target:', sorted(target, key=lambda p: -looked[task][p])[:8])
    print('   jev top5 need_read:', sorted(cands, key=lambda p: -ranks['need_read'][p])[:5])
print('totals', {m: f"{v[0]}@5 {v[1]}@10" for m, v in tot.items()})
