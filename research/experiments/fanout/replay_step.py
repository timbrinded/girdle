import json, sys, concurrent.futures as cf
from jevlib import ask
from bank import CURRENT, current_reqs, BROAD, broad_reqs
rows = json.load(open('step_rows.json'))
out_path = 'step_answers.jsonl'
done = set()
try:
    for l in open(out_path): done.add(json.loads(l)['i'])
except FileNotFoundError: pass
def flat(ans):
    o = {}
    for k, a in ans.items():
        if a['type'] == 'noul': o[k] = a['noul']
        elif a['type'] == 'score': o[k] = a['score']; o[k + '_probs'] = a.get('probabilities')
        else: o[k] = a.get('choice'); o[k + '_probs'] = a.get('probabilities')
    return o
def work(i):
    r = rows[i]
    reqs = r['state'].get('requirements') or []
    base = ask(r['state'], {**CURRENT, **current_reqs(reqs)})
    rich = ask(r['rich'], {**CURRENT, **current_reqs(reqs), **BROAD, **broad_reqs(reqs)})
    return {'i': i, 'base': flat(base['answers']), 'rich': flat(rich['answers']),
            'tok': base['usage']['input_tokens'] + rich['usage']['input_tokens'], 'ms': rich['_ms']}
todo = [i for i in range(len(rows)) if i not in done]
n = 0; errs = 0
with open(out_path, 'a') as f, cf.ThreadPoolExecutor(8) as ex:
    for fut in cf.as_completed([ex.submit(work, i) for i in todo]):
        try:
            f.write(json.dumps(fut.result()) + '\n'); n += 1
        except Exception as e:
            errs += 1; print('error', str(e)[:200], file=sys.stderr)
        if n % 200 == 0: f.flush(); print(n, 'done', file=sys.stderr)
print('replayed', n, 'errors', errs)
