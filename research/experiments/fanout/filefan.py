from jevlib import REPO
import json, glob, re, os, subprocess, collections, concurrent.futures as cf, sys
from jevlib import ask, noul, score
G = REPO + '/'
TASKS = ['gm-heading-close', 'gm-rename-indent', 'gm-strike-tag', 'mi-argsort', 'mi-unique-window', 'mi-window-deque']
# labels: files looked up, read or changed in passing fast and default runs
looked = collections.defaultdict(collections.Counter); changed = collections.defaultdict(collections.Counter); nruns = collections.Counter()
for f in glob.glob(G + 'bench/results/*/*/events.jsonl') + glob.glob(G + 'bench/results/*/*/*/events.jsonl'):
    try: r = json.load(open(f.replace('events.jsonl', 'result.json')))
    except Exception: continue
    if r['task'] not in TASKS or not r['pass'] or r['agent'] not in ('girdle-fast', 'girdle'): continue
    nruns[r['task']] += 1
    seen, ch = set(), set()
    for l in open(f):
        e = json.loads(l)
        if e.get('type') != 'tool_call': continue
        try: a = json.loads(e.get('input') or '{}')
        except Exception: continue
        if e['tool'] == 'lookup':
            fs = a.get('files') or []
            if isinstance(fs, str): fs = [fs]
            for p in fs: seen.add(str(p).split(':')[0])
        elif e['tool'] in ('read', 'view'):
            seen.add(str(a.get('path', '')))
        elif e['tool'] == 'apply':
            chs = a.get('changes') or []
            if isinstance(chs, str):
                try: chs = json.loads(chs)
                except Exception: chs = []
            for c in chs if isinstance(chs, list) else []:
                if isinstance(c, dict): ch.add(c.get('path', ''))
        elif e['tool'] in ('edit', 'write'):
            ch.add(str(a.get('path', '')))
    for p in seen: looked[r['task']][p.lstrip('./')] += 1
    for p in ch: changed[r['task']][p.lstrip('./')] += 1
def in_snapshot(task):
    t = open(f'snap-{task}.txt').read()
    return set(re.findall(r'<file path="([^"]+)">\n', t))
def repo(task): return G + f'bench/.warm/girdle-fast/{task}/repo'
def files(task):
    out = subprocess.run(['git', '-C', repo(task), 'ls-files'], capture_output=True, text=True).stdout.split()
    return [p for p in out if p.endswith(('.go', '.py', '.pyi', '.rst', '.md')) and not p.startswith(('_test_data', '.github'))]
def outline(task, p):
    try: txt = open(os.path.join(repo(task), p), errors='replace').read()
    except Exception: return ''
    lines = txt.splitlines()
    defs = [l.strip() for l in lines if re.match(r'^(func |type |def |class |    def )', l)][:60]
    head = '\n'.join(lines[:25])
    return head + '\n…\n' + '\n'.join(defs)
Q = {
  'need_read': noul("Will someone doing `task` need to read `file.path` to make the change, or to see how similar code in this repository is written?"),
  'will_change': noul("Will doing `task` need a change to `file.path`?"),
  'example': noul("Does `file` show an example of the pattern `task` needs, such as how a similar option, test, renderer or function is written?"),
  'relevance': score("How relevant is `file` to doing `task`?", ["irrelevant", "slightly", "clearly", "essential"]),
}
if __name__ == '__main__':
    res = {}
    jobs = []
    with cf.ThreadPoolExecutor(16) as ex:
        for task in TASKS:
            prompt = open(G + f'bench/scale/{task}/task.md').read().strip()
            for p in files(task):
                st = {'task': prompt, 'file': {'path': p, 'outline': outline(task, p)}}
                jobs.append((task, p, ex.submit(ask, st, Q)))
        for task, p, fut in jobs:
            try:
                a = fut.result()['answers']
                res.setdefault(task, {})[p] = {k: (v['noul'] if v['type'] == 'noul' else v['score']) for k, v in a.items()}
            except Exception as e:
                print('error', task, p, str(e)[:120], file=sys.stderr)
    json.dump({'res': res, 'looked': looked, 'changed': changed, 'nruns': nruns}, open('filefan.json', 'w'))
    print({t: len(v) for t, v in res.items()}, dict(nruns))
