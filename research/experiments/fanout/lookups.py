"""For scale runs: each lookup call, the files looked up so far, and the
files looked up later that weren't seen before."""
import json, glob, collections, random
from filefan import TASKS, in_snapshot, G
events = []
for f in glob.glob(G + 'bench/results/*/*/events.jsonl') + glob.glob(G + 'bench/results/*/*/*/events.jsonl'):
    try: r = json.load(open(f.replace('events.jsonl', 'result.json')))
    except Exception: continue
    if r['task'] not in TASKS or not r['pass'] or r['agent'] != 'girdle-fast' or 'fanout-live' in f: continue
    seq = []
    for l in open(f):
        e = json.loads(l)
        if e.get('type') == 'tool_call' and e['tool'] == 'lookup':
            try: a = json.loads(e.get('input') or '{}')
            except Exception: continue
            fs = a.get('files') or []
            if isinstance(fs, str): fs = [fs]
            seq.append(sorted({str(p).split(':')[0].lstrip('./') for p in fs}))
    snap = in_snapshot(r['task'])
    for k in range(len(seq)):
        seen = set().union(*seq[:k + 1]) if seq else set()
        later = set().union(*seq[k + 1:]) - seen - snap if k + 1 < len(seq) else set()
        if later:
            events.append({'task': r['task'], 'run': f, 'k': k, 'seen': sorted(seen), 'later': sorted(later)})
print(len(events), 'lookups followed by new files;', collections.Counter(e['task'] for e in events))
random.seed(1); random.shuffle(events)
json.dump(events, open('lookup_events.json', 'w'))
