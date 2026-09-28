import json, collections
rows = json.load(open('step_rows.json'))
A = {}
for l in open('step_answers.jsonl'):
    a = json.loads(l); A[a['i']] = a

def auc(pos, neg):
    if not pos or not neg: return float('nan')
    s = 0.0
    for p in pos:
        for n in neg:
            s += 1 if p > n else 0.5 if p == n else 0
    return s / (len(pos) * len(neg))

def feats(i):
    r, a = rows[i], A[i]
    b, x = a['base'], a['rich']
    reqs = r['state'].get('requirements') or []
    f = {}
    for k, v in x.items():
        if isinstance(v, (int, float)) and not k.startswith('req_') and not k.startswith('r'):
            f[k] = v
    for k in ('complete', 'complete_hidden', 'confidence', 'remaining', 'breadth'):
        if k in x: f[k] = x[k]
    f['base_complete'] = b.get('complete', 0)
    f['status_done_verified'] = (x.get('status_probs') or {}).get('done_verified', 0)
    f['status_done_any'] = sum((x.get('status_probs') or {}).get(k, 0) for k in ('done_verified', 'done_unverified'))
    f['next_stop'] = (x.get('next_probs') or {}).get('stop', 0)
    instr = [j for j in range(len(reqs)) if b.get(f'req_{j}_is_instruction', 1) >= 0.5]
    for name, key in (('min_req_base', 'req_{}'), ('min_r_tested', 'r{}_tested'), ('min_r_impl', 'r{}_impl'), ('min_r_shown', 'r{}_shown')):
        src = b if name == 'min_req_base' else x
        vals = [src.get(key.format(j)) for j in instr if src.get(key.format(j)) is not None]
        f[name] = min(vals) if vals else 1.0
    return f

def current_policy(i):
    r, b = rows[i], A[i]['base']
    f = feats(i)
    ta = r['tests_asked'] if r['tests_asked'] is not None else A[i]['rich'].get('tests_asked', 0)
    if b.get('complete', 0) < 0.7: return False
    if f['min_req_base'] < 0.5: return False
    if ta >= 0.5 and not r['tests_changed']: return False
    return True

F = {i: feats(i) for i in A}
stop = {i for i in A if current_policy(i)}
if __name__ == '__main__':
    c = collections.Counter((i in stop, rows[i]['safe']) for i in A)
    print(f"current policy on replay: stops {c[(True,True)]} safe + {c[(True,False)]} unsafe | continues {c[(False,True)]} safe + {c[(False,False)]} unsafe")
    names = sorted(next(iter(F.values())).keys())
    res = []
    for k in names:
        allp = [F[i][k] for i in A if rows[i]['safe']]; alln = [F[i][k] for i in A if not rows[i]['safe']]
        sp = [F[i][k] for i in stop if rows[i]['safe']]; sn = [F[i][k] for i in stop if not rows[i]['safe']]
        cp = [F[i][k] for i in A if i not in stop and rows[i]['safe']]; cn = [F[i][k] for i in A if i not in stop and not rows[i]['safe']]
        res.append((k, auc(allp, alln), auc(sp, sn), auc(cp, cn)))
    print(f"{'feature':<22} {'AUC all':>8} {'in stops':>9} {'in conts':>9}   (AUC for 'safe'; <0.5 means high = unsafe)")
    for k, a1, a2, a3 in sorted(res, key=lambda t: -abs(t[1] - 0.5)):
        print(f"{k:<22} {a1:8.3f} {a2:9.3f} {a3:9.3f}")
    json.dump({'stop': sorted(stop)}, open('current_stop.json', 'w'))
