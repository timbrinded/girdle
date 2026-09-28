import json, sys, concurrent.futures as cf, time
from jevlib import ask, noul
from filefan import TASKS, G, files, outline, in_snapshot
d = json.load(open('filefan.json'))
out = {}
t0 = time.time(); ms = []
def run(task, chunk):
    prompt = open(G + f'bench/scale/{task}/task.md').read().strip()
    st = {'task': prompt, 'files': [{'path': p, 'outline': outline(task, p)} for p in chunk]}
    qs = {f'f{k}': noul(f"Will someone doing `task` need to read `files[{k}]` to make the change, or to see how similar code in this repository is written?") for k in range(len(chunk))}
    r = ask(st, qs); ms.append(r['_ms'])
    return task, chunk, r['answers'], r['usage']['input_tokens']
jobs = []
with cf.ThreadPoolExecutor(16) as ex:
    for task in TASKS:
        snap = in_snapshot(task)
        cands = [p for p in files(task) if p not in snap and 'html5entities' not in p]
        for i in range(0, len(cands), 15):
            jobs.append(ex.submit(run, task, cands[i:i + 15]))
    toks = []
    for j in jobs:
        task, chunk, a, tk = j.result(); toks.append(tk)
        for k, p in enumerate(chunk): out.setdefault(task, {})[p] = a[f'f{k}']['noul']
print(f"{len(jobs)} requests, median {sorted(ms)[len(ms)//2]} ms, max tokens {max(toks)}")
json.dump(out, open('filefan_batch.json', 'w'))
# compare recall with the per-file answers
looked, nruns = d['looked'], d['nruns']
tot = {'per file': [0, 0], 'batched': [0, 0]}
for task in TASKS:
    snap = in_snapshot(task); cands = [p for p in out[task]]
    target = {p for p, c in looked[task].items() if c / nruns[task] >= 0.25 and p in cands}
    for name, sc in (('per file', {p: d['res'][task].get(p, {}).get('need_read', 0) for p in cands}), ('batched', out[task])):
        order = sorted(cands, key=lambda p: -sc[p])
        tot[name][0] += len(target & set(order[:5])); tot[name][1] += len(target & set(order[:10]))
print({k: f"{v[0]}@5 {v[1]}@10 of 21" for k, v in tot.items()})
