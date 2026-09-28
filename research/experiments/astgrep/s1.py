import json, re, subprocess, collections
from unitlabels import TASKS, repo, lang, G, uid
d = json.load(open('unitlabels.json')); U, looked, nruns = d['units'], d['looked'], d['nruns']
def named(task):
    t = open(G + f'bench/scale/{task}/task.md').read(); ns = []
    for m in re.findall(r'`([^`\n]+)`', t):
        m = m.split('(')[0].split('.')[-1].strip()
        if re.match(r'^[A-Za-z_][A-Za-z0-9_]{2,}$', m) and m not in ns: ns.append(m)
    return ns
def uses(task, name):
    out = subprocess.run(['rg', '-n', '-w', '--no-heading', name, '.'], cwd=repo(task), capture_output=True, text=True).stdout
    res = []
    for l in out.splitlines():
        p, n, _ = l.split(':', 2); res.append((p.lstrip('./'), int(n)))
    return res
def enclosing(task, p, line):
    return [u for u in U[task] if u['file'] == p and u['start'] <= line <= u['end']]
def callees(task, u):
    text = ''.join(open(f"{repo(task)}/{u['file']}", errors='replace').readlines()[u['start'] - 1:u['end']])
    names = set(re.findall(r'\b([A-Za-z_][A-Za-z0-9_]*)\s*\(', text))
    return [v for v in U[task] if any(v['head'].startswith(k) for k in ('def ', 'func ', 'class ')) and re.search(r'\b(def|func|class)\s+(\([^)]*\)\s*)?(' + '|'.join(map(re.escape, names)) + r')\b', v['head'])] if names else []
tot_w = tot_hit = 0
for task in TASKS:
    ns = named(task); picked = {}
    for n in ns:
        for p, line in uses(task, n):
            for u in enclosing(task, p, line):
                if u['bytes'] > 60: picked[uid(u)] = u
        # callees of the named definitions
        for u in [u for u in U[task] if re.search(r'\b(def|func|class|type)\s+(\([^)]*\)\s*)?' + re.escape(n) + r'\b', u['head'])]:
            for v in callees(task, u): picked[uid(v)] = v
    L = looked[task]; n = nruns[task]
    core = {k: c for k, c in L.items() if c / n >= 0.25}
    w = sum(core.values()); hit = sum(c for k, c in core.items() if k in picked)
    tot_w += w; tot_hit += hit
    size = sum(u['bytes'] for u in picked.values())
    print(f"{task:<18} names {ns[:5]} | picked {len(picked)} units, {size//1024} KB | covers {hit}/{w} weighted lookups of {len(core)} core units")
    missed = [k for k in core if k not in picked]
    if missed: print('    missed:', missed[:6])
print(f"total weighted coverage {tot_hit}/{tot_w} = {tot_hit/tot_w:.0%}")
