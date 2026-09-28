"""Build the step-end and failed-apply replay datasets from benchmark logs.

Step-end label: safe = the run passed and no later apply changed a file, so
stopping here would have handed back the same, passing code.
Apply label (failed applies only): what the agent's next changing apply
touched: tests only, code only, or both.
"""
from jevlib import REPO
import glob, json, os, re

ROOT = REPO + '/bench/results'

def is_test(p):
    b = os.path.basename(p)
    return b.endswith('_test.go') or b.startswith('test_') or '.test.' in b or '/tests/' in p or p.startswith('tests/') or 'zz_' in b

def parse_apply(inp):
    try:
        a = json.loads(inp)
    except Exception:
        return [], '', ''
    ch = a.get('changes') or []
    if isinstance(ch, str):
        try: ch = json.loads(ch)
        except Exception: ch = []
    if not isinstance(ch, list): ch = []
    return [c for c in ch if isinstance(c, dict)], a.get('check', '') or '', a.get('reproduce', '') or ''

def clip(s, n):
    s = s or ''
    return s if len(s) <= n else s[: n * 2 // 3] + ' … ' + s[-n // 3:]

def render(c):
    p = c.get('path', '?')
    if c.get('old_text') is not None and c.get('old_text') != '':
        return f"edit {p}\n- " + clip(c.get('old_text', ''), 600).replace('\n', '\n- ') + "\n+ " + clip(c.get('new_text', ''), 1500).replace('\n', '\n+ ')
    return f"write {p}\n" + clip(c.get('content', ''), 2500)

def exit_ok(text):
    m = re.search(r'\[exit code (\d+)\]', text or '')
    return m is not None and m.group(1) == '0'

step_rows, apply_rows = [], []
for rf in sorted(glob.glob(ROOT + '/*/*/result.json')):
    r = json.load(open(rf))
    if r.get('invalid') or not r['agent'].startswith('girdle'):
        continue
    ef = os.path.join(os.path.dirname(rf), 'events.jsonl')
    if not os.path.exists(ef):
        continue
    E = []
    for l in open(ef):
        try: E.append(json.loads(l))
        except Exception: pass
    run = os.path.relpath(os.path.dirname(rf), ROOT)
    tests_asked = next((e['route'].get('tests') for e in E if e.get('type') == 'route' and e.get('route')), None)
    calls = {}
    changes, files = [], []
    last_check, last_out = '', ''
    change_idx = []  # event index of every apply/edit/write that changed a file
    for i, e in enumerate(E):
        if e.get('type') == 'tool_call' and e.get('tool') in ('apply', 'edit', 'write'):
            if e['tool'] == 'apply':
                ch, _, _ = parse_apply(e.get('input', ''))
                if ch: change_idx.append(i)
            else:
                change_idx.append(i)
    for i, e in enumerate(E):
        t = e.get('type')
        if t == 'tool_call':
            calls[e.get('call_id')] = e
        elif t == 'tool_result' and e.get('tool') == 'apply':
            call = calls.get(e.get('call_id'), {})
            ch, check, repro = parse_apply(call.get('input', ''))
            text = e.get('text', '')
            ok = exit_ok(text) and 'reproduce command also passes' not in text
            if not ok and check:
                # the next changing apply after this one
                nxt = next((j for j in change_idx if j > i), None)
                nxt_paths = []
                if nxt is not None:
                    nch, _, _ = parse_apply(E[nxt].get('input', '')) if E[nxt].get('tool') == 'apply' else ([{'path': json.loads(E[nxt].get('input') or '{}').get('path', '')}], '', '')
                    nxt_paths = [c.get('path', '') for c in nch]
                label = None
                if nxt_paths:
                    t_only = all(is_test(p) for p in nxt_paths)
                    c_only = not any(is_test(p) for p in nxt_paths)
                    label = 'tests' if t_only else 'code' if c_only else 'both'
                apply_rows.append({
                    'run': run, 'task': r['task'], 'agent': r['agent'], 'pass': r['pass'], 'label': label,
                    'state': {'task': (next((x.get('text') for x in E if x.get('type') == 'user_message'), '') or ''),
                              'changes_so_far': [render(c) for c in changes + ch][-12:],
                              'check': check, 'check_output': clip(text, 3500)},
                })
            for c in ch:
                changes.append(c); files.append(c.get('path', ''))
            if check:
                last_check, last_out = check, text
        elif t == 'decision' and e['decision'].get('checkpoint') == 'step_end':
            d = e['decision']
            later = any(j > i for j in change_idx)
            safe = bool(r['pass']) and not later
            st = d.get('state') or {}
            rich = dict(st)
            rich['changes'] = [render(c) for c in changes][-12:]
            rich['files_changed'] = sorted(set(files))
            rich['tests_changed'] = sorted({p for p in files if is_test(p)})
            rich['check'] = last_check
            rich['check_output'] = clip(last_out, 3500)
            step_rows.append({
                'run': run, 'task': r['task'], 'agent': r['agent'], 'pass': r['pass'], 'safe': safe,
                'action': d.get('action'), 'answers': {k: v.get('noul') for k, v in (d.get('answers') or {}).items()},
                'tests_asked': tests_asked, 'tests_changed': bool(rich['tests_changed']),
                'state': st, 'rich': rich,
            })
json.dump(step_rows, open('step_rows.json', 'w'))
json.dump(apply_rows, open('apply_rows.json', 'w'))
import collections
print('step rows', len(step_rows), collections.Counter((r['action'], r['safe']) for r in step_rows))
print('apply rows', len(apply_rows), collections.Counter(r['label'] for r in apply_rows))
print('tasks', len({r['task'] for r in step_rows}))
