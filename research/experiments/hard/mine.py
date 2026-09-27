import json, os, re, subprocess, sys
def is_test(p): b = os.path.basename(p); return b.endswith('_test.go') or b.startswith('test_') or '/tests/' in p or p.startswith('tests/') or '/testdata/' in p
def is_src(p): return p.endswith(('.go', '.py')) and not is_test(p) and not p.startswith(('docs/', 'examples/', '_examples/', 'cmd/', 'internal/tools'))
SKIP = re.compile(r'(bump|deps|dependabot|typo|docs?:|readme|lint|format|gofmt|ci:|workflow|release|version|changelog|refactor|cleanup|style)', re.I)
out = []
for repo in sys.argv[1:]:
    log = subprocess.run(['git', '-C', 'repos/' + repo, 'log', '--no-merges', '--format=%x00%H %cs %s', '--numstat', '--since=2016-01-01'], capture_output=True, text=True).stdout
    for chunk in log.split('\x00')[1:]:
        lines = chunk.strip().split('\n')
        h, date, subj = lines[0].split(' ', 2)
        if SKIP.search(subj): continue
        src = tests = 0; srcfiles = []; testfiles = []
        for l in lines[1:]:
            parts = l.split('\t')
            if len(parts) != 3 or parts[0] == '-': continue
            n = int(parts[0]) + int(parts[1]); p = parts[2]
            if is_test(p): tests += n; testfiles.append(p)
            elif is_src(p): src += n; srcfiles.append(p)
        if srcfiles and testfiles and 10 <= src <= 250 and tests >= 5:
            out.append({'repo': repo, 'hash': h, 'date': date, 'subject': subj, 'src_lines': src, 'src_files': srcfiles, 'test_files': testfiles})
json.dump(out, open('candidates.json', 'w'), indent=1)
import collections
print(collections.Counter(c['repo'] for c in out))
