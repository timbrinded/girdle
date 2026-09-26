from jevlib import REPO
import json, re, collections, concurrent.futures as cf, sys
from jevlib import ask, noul
T = REPO + '/bench/tasks/'
SRC = {'js-slugify': 'slug.js', 'js-csv': 'csv.js', 'py-duration': 'durations.py', 'go-ttl-cache': 'cache.go'}
def bullets(path):
    items, cur = [], None
    for raw in open(path):
        l = raw.strip()
        if not (l.startswith(('*', '//', '-')) or l.startswith('"""') or raw.startswith('    ') or raw.startswith('//')):
            if cur: items.append(cur); cur = None
            continue
        l = re.sub(r'^(/\*\*|\*/|\*|//|""")\s?', '', l).strip()
        if l.startswith('- '):
            if cur: items.append(cur)
            cur = l[2:]
        elif cur and l:
            cur += ' ' + l
        elif cur and not l:
            items.append(cur); cur = None
    if cur: items.append(cur)
    return items
SPEC = {t: bullets(T + t + '/repo/' + f) for t, f in SRC.items()}
for t, s in SPEC.items(): print(t, len(s), 'rules')
rows = json.load(open('step_rows.json'))
ids = [i for i, r in enumerate(rows) if r['task'] in SPEC]
def work(i):
    r = rows[i]; spec = SPEC[r['task']]
    st = dict(r['rich']); st['spec'] = spec
    qs = {}
    for k in range(len(spec)):
        qs[f's{k}_impl'] = noul(f"Does the code in `changes` follow `spec[{k}]` exactly, in every detail it states?")
        qs[f's{k}_tested'] = noul(f"Does a test in `changes` check `spec[{k}]`?")
    qs['any_rule_broken'] = noul("Does the code in `changes` break any rule in `spec`?")
    qs['all_rules_tested'] = noul("Is every rule in `spec` checked by some test in `changes`?")
    a = ask(st, qs)['answers']
    return i, {k: v['noul'] for k, v in a.items()}
out = {}
with cf.ThreadPoolExecutor(8) as ex:
    for fut in cf.as_completed([ex.submit(work, i) for i in ids]):
        i, o = fut.result(); out[i] = o
json.dump({'spec': SPEC, 'answers': out}, open('spec_answers.json', 'w'))
def auc(pos, neg):
    s = sum(1 if p > n else 0.5 if p == n else 0 for p in pos for n in neg); return s / (len(pos) * len(neg)) if pos and neg else float('nan')
from analyse_step import F, current_policy
cur = {i for i in ids if current_policy(i)}
print('rows', len(ids), 'safe', sum(rows[i]['safe'] for i in ids), '| current stops', len(cur), 'unsafe stops', sum(not rows[i]['safe'] for i in cur))
def mins(i, suf):
    return min(out[i][f's{k}_{suf}'] for k in range(len(SPEC[rows[i]['task']])))
feat = {
  'min spec rule implemented': lambda i: mins(i, 'impl'),
  'min spec rule tested': lambda i: mins(i, 'tested'),
  'any rule broken (neg)': lambda i: 1 - out[i]['any_rule_broken'],
  'all rules tested': lambda i: out[i]['all_rules_tested'],
  'min requirement implemented': lambda i: F[i]['min_r_impl'],
  'status done_verified': lambda i: F[i]['status_done_verified'],
  'current complete (logged state)': lambda i: F[i]['base_complete'],
}
print(f"{'feature':<34} {'AUC all':>8} {'in stops':>9}")
for n, fn in feat.items():
    print(f"{n:<34} {auc([fn(i) for i in ids if rows[i]['safe']], [fn(i) for i in ids if not rows[i]['safe']]):8.3f} {auc([fn(i) for i in cur if rows[i]['safe']], [fn(i) for i in cur if not rows[i]['safe']]):9.3f}")
for thr in (0.3, 0.4, 0.5, 0.6):
    veto = [i for i in cur if mins(i, 'impl') < thr]
    print(f"veto stops where a rule's implemented < {thr}: {len(veto)} vetoed, {sum(not rows[i]['safe'] for i in veto)} of them unsafe")
