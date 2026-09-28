"""Top-level structural units of source files, via ast-grep."""
import json, subprocess, os
RULES = {
    'go': "id: u\nlanguage: go\nrule:\n  any:\n    - kind: function_declaration\n    - kind: method_declaration\n    - kind: type_declaration\n    - kind: var_declaration\n    - kind: const_declaration\n  inside:\n    kind: source_file\n    stopBy: neighbor\n",
    'python': "id: u\nlanguage: python\nrule:\n  any:\n    - kind: function_definition\n    - kind: class_definition\n    - kind: decorated_definition\n    - kind: expression_statement\n  inside:\n    kind: module\n    stopBy: neighbor\n",
}
EXT = {'.go': 'go', '.py': 'python', '.pyi': 'python'}
def units(repo, lang):
    out = subprocess.run(['ast-grep', 'scan', '--inline-rules', RULES[lang], '--json=compact', '.'], cwd=repo, capture_output=True, text=True).stdout
    res = []
    for m in json.loads(out or '[]'):
        text = m['text']
        first = text.split('\n', 1)[0]
        res.append({'file': m['file'], 'start': m['range']['start']['line'] + 1, 'end': m['range']['end']['line'] + 1, 'head': first[:160], 'bytes': len(text)})
    return res
if __name__ == '__main__':
    import sys, time
    t = time.time(); u = units(sys.argv[1], sys.argv[2]); print(len(u), 'units in', round(time.time() - t, 2), 's')
    for x in u[:5]: print(x)
    big = [x for x in u if x['file'].endswith('test_more.py')]
    print(len(big), 'in test_more.py; e.g.', [x for x in big if x['start'] <= 5707 <= x['end']])
