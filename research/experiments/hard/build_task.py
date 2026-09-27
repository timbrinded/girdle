"""Build a bench/hard task from an upstream fix commit, SWE-bench style."""
import os, pathlib, re, stat, subprocess, sys
G = str(pathlib.Path(__file__).resolve().parents[3]) + '/'
ORG = {'toml': 'BurntSushi', 'go-cmp': 'google', 'semver': 'Masterminds', 'pflag': 'spf13', 'go-version': 'hashicorp',
       'cron': 'robfig', 'goldmark': 'yuin', 'more-itertools': 'more-itertools', 'expr': 'expr-lang', 'mux': 'gorilla'}
def is_test_path(p):
    b = os.path.basename(p)
    return (b.endswith('_test.go') or b.startswith('test_') or p.startswith(('tests/', '_test/', 'testdata/'))
            or '/testdata/' in p or '/_test/' in p or '/tests/' in p)
def git(repo, *a): return subprocess.run(['git', '-C', 'repos/' + repo, *a], capture_output=True, text=True).stdout
def build(repo, h, name):
    parent = git(repo, 'rev-parse', h + '^').strip()
    files = [f for f in git(repo, 'diff', '--name-only', parent, h).split('\n') if f]
    tests = [f for f in files if is_test_path(f)]
    src = [f for f in files if not is_test_path(f)]
    d = G + f'bench/hard/{name}'
    os.makedirs(d + '/hidden', exist_ok=True)
    open(d + '/source', 'w').write(f"https://github.com/{ORG[repo]}/{repo}.git {parent}\n")
    open(d + '/solution.patch', 'w').write(git(repo, 'diff', '--binary', parent, h, '--', *src))
    open(d + '/hidden/tests.patch', 'w').write(git(repo, 'diff', '--binary', parent, h, '--', *tests))
    go = any(f.endswith('.go') for f in files)
    if go:
        # Directories inside a nested module aren't this module's packages.
        tree = git(repo, 'ls-tree', '-r', '--name-only', h).split('\n')
        nested = {os.path.dirname(f) for f in tree if f.endswith('/go.mod')}
        pkgs = sorted({'./' + (os.path.dirname(f) or '.') for f in files if f.endswith('.go')
                       and not any((os.path.dirname(f) + '/').startswith(n + '/') for n in nested)})
        # Vet is off: some upstream commits carry unrelated vet failures.
        # goldmark's timing tests fail under benchmark load.
        skip = ' -skip Performance' if repo == 'goldmark' else ''
        run = f'go test -vet=off{skip} {" ".join(pkgs)}'
        reset = "git ls-files -z -- '*_test.go' '*/testdata/*' 'testdata/*' '_test/*' '*/_test/*' | xargs -0 git checkout -q HEAD -- 2>/dev/null || true\ngit ls-files -z --others --exclude-standard -- '*_test.go' | xargs -0 rm -f"
    else:
        run = 'python3 -m unittest discover -s tests -t .'
        reset = "git checkout -q HEAD -- tests 2>/dev/null || true\ngit ls-files -z --others --exclude-standard -- tests | xargs -0 rm -f"
    open(d + '/check.sh', 'w').write(f"""set -e
# SWE-bench style: the agent's own test changes are set aside, and the
# upstream tests from the fix commit decide.
{reset}
git apply "$TASK_DIR/hidden/tests.patch"
{run}
""")
    if not os.path.exists(d + '/task.md'):
        subj = git(repo, 'log', '-1', '--format=%B', h).strip()
        open(d + '/task.md', 'w').write(f"TODO: write the task. Upstream commit message:\n\n{subj}\n")
    print(name, parent[:10], 'src', src, 'tests', tests, run)
if __name__ == '__main__':
    for spec in sys.argv[1:]:
        repo, h, name = spec.split(':')
        build(repo, h, name)
