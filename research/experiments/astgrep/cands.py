"""Structural candidates (ast-grep units) and Jev's ranking of them."""
import json, re, subprocess, collections, sys, os, concurrent.futures as cf
from unitlabels import TASKS, repo, G, uid
from jevlib import ask, noul
d = json.load(open('unitlabels.json')); U, looked, nruns = d['units'], d['looked'], d['nruns']
DEF = lambda n: re.compile(r'^\s*(export\s+)?(def|func|class|type)\s+(\([^)]*\)\s*)?' + re.escape(n) + r'\b')
def text(task, u, clip=None):
    lines = open(f"{repo(task)}/{u['file']}", errors='replace').readlines()[u['start'] - 1:u['end']]
    t = ''.join(lines)
    return t if clip is None or len(t) <= clip else t[:clip * 2 // 3] + '\n…\n' + t[-clip // 3:]
def named(task):
    t = open(G + f'bench/scale/{task}/task.md').read(); ns = []
    for m in re.findall(r'`([^`\n]+)`', t):
        m = m.split('(')[0].split('.')[-1].strip()
        if re.match(r'^[A-Za-z_][A-Za-z0-9_]{2,}$', m) and m not in ns: ns.append(m)
    return ns
def defs(task, n): return [u for u in U[task] if DEF(n).search(u['head'])]
def uses(task, name):
    out = subprocess.run(['rg', '-n', '-w', '--no-heading', name, '.'], cwd=repo(task), capture_output=True, text=True).stdout
    return [(l.split(':', 2)[0].lstrip('./'), int(l.split(':', 2)[1])) for l in out.splitlines()]
def is_test(p): b = os.path.basename(p); return b.endswith('_test.go') or b.startswith('test_') or '/tests/' in p or p.startswith('tests/')
DEFN = re.compile(r'^\s*(?:export\s+)?(?:def|func|class|type)\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)')
_idx = {}
def defindex(task):
    if task not in _idx:
        m = collections.defaultdict(list)
        for u in U[task]:
            x = DEFN.match(u['head'])
            if x: m[x.group(1)].append(u)
        _idx[task] = m
    return _idx[task]
def referenced_defs(task, u):
    idx = defindex(task)
    names = set(re.findall(r'\b([A-Za-z_][A-Za-z0-9_]{2,})\b', text(task, u)))
    return [v for n in names for v in idx.get(n, []) if v is not u]
def called_defs(task, u):
    names = set(re.findall(r'\b([A-Za-z_][A-Za-z0-9_]*)\s*\(', text(task, u)))
    names -= {'if', 'for', 'switch', 'return', 'func', 'print', 'len', 'range', 'list', 'dict', 'set', 'str', 'int', 'make', 'append', 'super'}
    out = []
    for n in names:
        out += [v for v in defs(task, n) if v is not u]
    return out
def candidates(task):
    c = {}; shown = set()
    ns = [n for n in named(task) if defs(task, n)]
    for n in ns:
        for u in defs(task, n):
            shown.add(uid(u))                     # the snapshot already shows these
            for v in called_defs(task, u): c[uid(v)] = (v, 'callee')
        for p, line in uses(task, n):
            for u in U[task]:
                if u['file'] == p and u['start'] <= line <= u['end'] and u['bytes'] > 60:
                    c.setdefault(uid(u), (u, 'use'))
        # test helpers: what the tests beside the named code call
        for u in defs(task, n):
            base = os.path.dirname(u['file'])
            for t in U[task]:
                if is_test(t['file']) and os.path.dirname(t['file']) == base and t['head'].startswith(('func Test', 'class ')):
                    for v in referenced_defs(task, t):
                        if not is_test(v['file']) and os.path.dirname(v['file']) != base:
                            c.setdefault(uid(v), (v, 'test helper'))
                            # one step further inside the helper's own file
                            for w in referenced_defs(task, v):
                                if w['file'] == v['file']: c.setdefault(uid(w), (w, 'test helper'))
    for k in shown: c.pop(k, None)
    return c, shown
Q = {'need': noul("Will someone doing `task` need to read `unit` to make the change, or to see how to write the change or its tests?")}
if __name__ == '__main__':
    out = {}
    with cf.ThreadPoolExecutor(24) as ex:
        futs = {}
        for task in TASKS:
            c, shown = candidates(task)
            prompt = open(G + f'bench/scale/{task}/task.md').read().strip()
            for k, (u, why) in c.items():
                st = {'task': prompt, 'unit': {'file': u['file'], 'lines': f"{u['start']}-{u['end']}", 'text': text(task, u, 3000)}}
                futs[ex.submit(ask, st, Q)] = (task, k, why, u['bytes'])
            out[task] = {'shown': sorted(shown), 'cands': {}}
        for f in cf.as_completed(futs):
            task, k, why, b = futs[f]
            try: out[task]['cands'][k] = {'why': why, 'bytes': b, 'need': f.result()['answers']['need']['noul']}
            except Exception as e: print('error', str(e)[:100], file=sys.stderr)
    json.dump(out, open('cands.json', 'w'))
    for task in TASKS: print(task, len(out[task]['cands']), 'candidates;', collections.Counter(v['why'] for v in out[task]['cands'].values()))
