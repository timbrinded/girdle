import json, sys, collections, concurrent.futures as cf
from jevlib import ask, noul, choice, score
rows = json.load(open('apply_rows.json'))
Q = {
  "test_at_fault": noul("Is the failure in `check_output` caused by a mistake in a test rather than in the code the test checks?"),
  "expectation_contradicts_task": noul("Does a failing test in `check_output` expect a value that contradicts the words of `task`?"),
  "expectation_follows_task": noul("Does the failing test's expectation follow from the words of `task`, so that the code must be what's wrong?"),
  "fix_where": choice("Where should the fix for the failure in `check_output` go?", {"tests": "only in the tests", "code": "only in the code under test", "both": "in both the tests and the code"}),
  "kind": choice("What kind of failure does `check_output` show?", {
     "build": "a build, compile, syntax or import error", "assertion": "a test assertion comparing values failed",
     "runtime": "a crash, panic or exception at run time", "timeout": "a timeout or hang",
     "env": "a missing tool, dependency or environment problem", "flaky": "a timing or performance flake", "check": "the check command itself is wrong"}),
  "missing_name": noul("Does the failure come from using a function, method, field or module that doesn't exist?"),
  "unrelated": noul("Is the failure in a test that has nothing to do with `task`?"),
  "new_test_failed": noul("Is the failing test one that the agent added or changed in `changes_so_far`?"),
  "old_test_failed": noul("Is the failing test one that existed before `changes_so_far`?"),
  "one_line_fix": noul("Could the failure be fixed by a change of a line or two?"),
  "severity": score("How far is the code from working?", ["one small fix away", "a few fixes away", "a larger rework away", "fundamentally wrong"]),
}
def work(i):
    r = rows[i]
    a = ask(r['state'], Q)['answers']
    o = {}
    for k, v in a.items():
        o[k] = v['noul'] if v['type'] == 'noul' else v.get('score') if v['type'] == 'score' else v.get('choice')
        if v['type'] != 'noul': o[k + '_probs'] = v.get('probabilities')
    return i, o
out = {}
with cf.ThreadPoolExecutor(8) as ex:
    for fut in cf.as_completed([ex.submit(work, i) for i in range(len(rows))]):
        try:
            i, o = fut.result(); out[i] = o
        except Exception as e:
            print('error', str(e)[:200], file=sys.stderr)
json.dump(out, open('apply_answers.json', 'w'))
print('replayed', len(out))
