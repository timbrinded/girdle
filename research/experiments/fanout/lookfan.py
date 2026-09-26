"""After each lookup, can Jev predict the files the agent will look up later?
One request per candidate file; the state says what was looked up so far."""
import json, random, sys, concurrent.futures as cf
from jevlib import ask, noul
from filefan import files, outline, in_snapshot, G
ev = json.load(open('lookup_events.json'))
# every non-strike event, plus 30 strike-tag ones
sample = [e for e in ev if e['task'] != 'gm-strike-tag'] + [e for e in ev if e['task'] == 'gm-strike-tag'][:30]
Q = {'need': noul("The agent has already read the files in `looked_up`. Will it still need to read `file.path` to finish `task`, for example to see how similar code or tests in this repository are written?")}
def work(j):
    e = sample[j]; task = e['task']
    prompt = open(G + f'bench/scale/{task}/task.md').read().strip()
    snap = in_snapshot(task)
    cands = [p for p in files(task) if p not in snap and p not in e['seen'] and 'html5entities' not in p]
    out = {}
    for p in cands:
        try:
            a = ask({'task': prompt, 'looked_up': e['seen'], 'file': {'path': p, 'outline': outline(task, p)}}, Q)
            out[p] = a['answers']['need']['noul']
        except Exception as ex:
            print('error', str(ex)[:100], file=sys.stderr)
    return j, out
if __name__ == '__main__':
    res = {}
    with cf.ThreadPoolExecutor(int(sys.argv[1]) if len(sys.argv) > 1 else 4) as ex:
        for j, out in ex.map(work, range(len(sample))):
            res[j] = out
    json.dump({'sample': sample, 'scores': res}, open('lookfan.json', 'w'))
    hits3 = hits5 = total = 0
    for j, e in enumerate(sample):
        order = sorted(res[j], key=lambda p: -res[j][p])
        later = set(e['later'])
        hits3 += len(later & set(order[:3])); hits5 += len(later & set(order[:5])); total += len(later)
    print(f"{len(sample)} lookups; files looked up later: {total}; in Jev's top 3: {hits3}; top 5: {hits5}")
