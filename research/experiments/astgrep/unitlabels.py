"""Label the structural units agents looked up, per scale task."""
from jevlib import REPO
import json, glob, collections, os, re, subprocess
from units import units
G = REPO + '/'
TASKS = ['gm-heading-close', 'gm-rename-indent', 'gm-strike-tag', 'mi-argsort', 'mi-unique-window', 'mi-window-deque']
def repo(t): return G + f'bench/.warm/girdle-fast/{t}/repo'
def lang(t): return 'go' if t.startswith('gm') else 'python'
U = {t: units(repo(t), lang(t)) for t in TASKS} if __name__ == '__main__' else json.load(open('unitlabels.json'))['units']
byfile = {t: collections.defaultdict(list) for t in TASKS}
for t in TASKS:
    for u in U[t]: byfile[t][u['file']].append(u)
def uid(u): return f"{u['file']}:{u['start']}"
looked = {t: collections.Counter() for t in TASKS}; nruns = collections.Counter()
for f in ([] if __name__ != '__main__' else  glob.glob(G + 'bench/results/*/*/events.jsonl') + glob.glob(G + 'bench/results/*/*/*/events.jsonl')):
    try: r = json.load(open(f.replace('events.jsonl', 'result.json')))
    except Exception: continue
    if r['task'] not in TASKS or not r['pass'] or not r['agent'].startswith('girdle-fast'): continue
    nruns[r['task']] += 1; seen = set()
    for l in open(f):
        e = json.loads(l)
        if e.get('type') != 'tool_call' or e['tool'] != 'lookup': continue
        try: a = json.loads(e['input'])
        except Exception: continue
        fs = a.get('files') or []
        if isinstance(fs, str): fs = [fs]
        for x in fs:
            x = str(x); m = re.match(r'^(.*?):(\d+)-(\d+)$', x)
            if not m: continue
            p, s, e2 = m.group(1).lstrip('./'), int(m.group(2)), int(m.group(3))
            for u in byfile[r['task']].get(p, []):
                ov = min(u['end'], e2) - max(u['start'], s) + 1
                # the range mostly covers the unit, or the unit mostly covers the range
                if u['bytes'] > 60 and ov > 0 and (ov >= 0.5 * (u['end'] - u['start'] + 1) or ov >= 0.5 * (e2 - s + 1)):
                    seen.add(uid(u))
    for x in seen: looked[r['task']][x] += 1
if __name__ == '__main__': json.dump({'units': U, 'looked': looked, 'nruns': nruns}, open('unitlabels.json', 'w'))
for t in (TASKS if __name__ == '__main__' else []):
    top = looked[t].most_common(8)
    heads = {uid(u): u['head'][:60] for u in U[t]}
    print(t, nruns[t], 'runs;', len(looked[t]), 'units looked up')
    for k, c in top: print(f"   {c:>3} {k:<40} {heads.get(k,'')}")
