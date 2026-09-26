import json, glob, collections, statistics as st, sys
root = sys.argv[1]
rows = collections.defaultdict(list)
for f in glob.glob(root + '/*/result.json'):
    r = json.load(open(f))
    E = [json.loads(l) for l in open(f.replace('result.json', 'events.jsonl'))]
    tools = [e['tool'] for e in E if e['type'] == 'tool_call']
    steps = [e for e in E if e['type'] == 'step' and (e.get('meta') or {}).get('role') != 'crosscheck']
    first = steps[0].get('duration_ms', 0) / 1000 if steps else 0
    snap = next((e['meta'] for e in E if e['type'] == 'snapshot'), {})
    se = [e['decision'] for e in E if e['type'] == 'decision' and e['decision']['checkpoint'] == 'step_end']
    rows[(r['task'], r['agent'])].append(dict(p=r['pass'], secs=r['secs'], cost=r['cost_usd'], look=tools.count('lookup'), steps=len(steps),
        first=first, pms=int(snap.get('prefetch_ms', 0) or 0), early=r.get('reason') == 'done_early', se=len(se)))
agents = sorted({a for _, a in rows})
tasks = sorted({t for t, _ in rows})
def agg(rs, k): return st.mean([x[k] for x in rs]) if rs else float('nan')
print(f"{'task':<18}" + ''.join(f"{a:>34}" for a in agents))
for t in tasks:
    cells = []
    for a in agents:
        rs = rows[(t, a)]
        cells.append(f"{sum(x['p'] for x in rs)}/{len(rs)} {agg(rs,'secs'):5.1f}s look {agg(rs,'look'):3.1f} st {agg(rs,'steps'):3.1f}")
    print(f"{t:<18}" + ''.join(f"{c:>34}" for c in cells))
for a in agents:
    for suite, pre in (('scale', ('gm-', 'mi-')), ('small', ('go-', 'py-', 'js-'))):
        rs = [x for (t, b), v in rows.items() if b == a and t.startswith(pre) for x in v]
        if not rs: continue
        print(f"{a:<24} {suite:<6} pass {sum(x['p'] for x in rs)}/{len(rs)}  mean {agg(rs,'secs'):5.1f}s  median {st.median([x['secs'] for x in rs]):5.1f}s  lookups {agg(rs,'look'):.2f}  LLM steps {agg(rs,'steps'):.2f}  first call {agg(rs,'first'):.1f}s  prefetch {agg(rs,'pms')/1000:.2f}s  early stops {sum(x['early'] for x in rs)}  cost ${agg(rs,'cost'):.4f}")
