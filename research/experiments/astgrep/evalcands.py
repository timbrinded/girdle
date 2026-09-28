import json
from unitlabels import TASKS
d = json.load(open('unitlabels.json')); looked, nruns = d['looked'], d['nruns']
C = json.load(open('cands.json'))
tot = {'all candidates': [0, 0, 0], 'jev top, 16 KB': [0, 0, 0], 'jev >= 0.7, 16 KB': [0, 0, 0], 'candidate order, 16 KB': [0, 0, 0]}
for task in TASKS:
    c = C[task]['cands']; shown = set(C[task]['shown'])
    core = {k: v for k, v in looked[task].items() if v / nruns[task] >= 0.25 and k not in shown}
    W = sum(core.values())
    def pick(order, minimum=0):
        out, b = [], 0
        for k in order:
            if c[k]['need'] < minimum: continue
            if b + c[k]['bytes'] > 16384: continue
            out.append(k); b += c[k]['bytes']
        return out, b
    sets = {
        'all candidates': (list(c), sum(v['bytes'] for v in c.values())),
        'jev top, 16 KB': pick(sorted(c, key=lambda k: -c[k]['need'])),
        'jev >= 0.7, 16 KB': pick(sorted(c, key=lambda k: -c[k]['need']), 0.7),
        'candidate order, 16 KB': pick(list(c)),
    }
    cells = []
    for name, (ks, b) in sets.items():
        hit = sum(v for k, v in core.items() if k in ks)
        tot[name][0] += hit; tot[name][1] += W; tot[name][2] += b
        cells.append(f"{name}: {hit}/{W} ({b // 1024} KB)")
    print(f"{task:<17} " + ' | '.join(cells))
print({k: f"{v[0]}/{v[1]} = {v[0] / max(v[1], 1):.0%}, {v[2] // 1024} KB total" for k, v in tot.items()})
