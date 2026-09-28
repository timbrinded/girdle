import json, collections
rows = json.load(open('apply_rows.json')); A = {int(k): v for k, v in json.load(open('apply_answers.json')).items()}
def auc(pos, neg):
    s = sum(1 if p > n else 0.5 if p == n else 0 for p in pos for n in neg); return s / (len(pos) * len(neg)) if pos and neg else float('nan')
lab = {i: rows[i]['label'] for i in A if rows[i]['label'] in ('tests', 'code') and rows[i]['pass']}
print('labelled (run passed, fix went only to tests or only to code):', collections.Counter(lab.values()))
feats = ['test_at_fault', 'expectation_contradicts_task', 'expectation_follows_task', 'missing_name', 'unrelated', 'new_test_failed', 'old_test_failed', 'one_line_fix', 'severity']
for k in feats:
    pos = [A[i][k] for i in lab if lab[i] == 'tests']; neg = [A[i][k] for i in lab if lab[i] == 'code']
    print(f"{k:<30} AUC(fix went to tests) {auc(pos, neg):.3f}")
pos = [(A[i].get('fix_where_probs') or {}).get('tests', 0) for i in lab if lab[i] == 'tests']; neg = [(A[i].get('fix_where_probs') or {}).get('tests', 0) for i in lab if lab[i] == 'code']
print(f"{'fix_where P(tests)':<30} AUC {auc(pos, neg):.3f}")
print('fix_where argmax vs actual:', collections.Counter((A[i]['fix_where'], lab[i]) for i in lab))
print('kind:', collections.Counter(A[i]['kind'] for i in A))
# confident calls
for t in (0.6, 0.7, 0.8):
    sel = [i for i in lab if A[i]['test_at_fault'] >= t]
    print(f"test_at_fault >= {t}: {len(sel)} calls, {sum(lab[i]=='tests' for i in sel)} right")
