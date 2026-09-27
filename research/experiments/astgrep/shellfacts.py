"""Facts about a shell command line, from ast-grep's bash parse."""
import json, os, re, subprocess
RULES = """id: cmd
language: bash
rule:
  kind: command
  has:
    field: name
    pattern: $NAME
---
id: redirect
language: bash
rule:
  kind: file_redirect
"""
def words(s):
    """Split a command's text into words, honouring quotes and escapes."""
    out, cur, q, i, any_ = [], '', None, 0, False
    while i < len(s):
        c = s[i]
        if q:
            if c == q: q = None
            elif c == '\\' and q == '"' and i + 1 < len(s): i += 1; cur += s[i]
            else: cur += c
        elif c in '"\'': q = c; any_ = True
        elif c == '\\' and i + 1 < len(s): i += 1; cur += s[i]; any_ = True
        elif c.isspace():
            if cur or any_: out.append(cur); cur = ''; any_ = False
        else: cur += c
        i += 1
    if cur or any_: out.append(cur)
    return out
def parse(line, depth=0):
    r = subprocess.run(['ast-grep', 'scan', '--inline-rules', RULES, '--json=stream', '--stdin'], input=line, capture_output=True, text=True)
    cmds, redirects = [], []
    for l in r.stdout.splitlines():
        m = json.loads(l)
        if m['ruleId'] == 'redirect':
            redirects.append(m['text'])
            continue
        w = words(m['text'])
        # a command's own redirects are part of its text in some grammars; drop them
        cmds.append(w)
        # bash -c "..." / sh -c / eval: parse the string too
        if depth < 3 and w and os.path.basename(w[0]) in ('bash', 'sh', 'zsh', 'dash') and '-c' in w:
            i = w.index('-c')
            if i + 1 < len(w):
                c2, r2 = parse(w[i + 1], depth + 1); cmds += c2; redirects += r2
        if depth < 3 and w and w[0] == 'eval' and len(w) > 1:
            c2, r2 = parse(' '.join(w[1:]), depth + 1); cmds += c2; redirects += r2
    return cmds, redirects
DELETERS = {'rm', 'rmdir', 'unlink', 'shred', 'srm'}
WRAPPERS = {'sudo', 'doas', 'env', 'nohup', 'time', 'nice', 'ionice', 'timeout', 'command', 'exec', 'builtin', 'stdbuf'}
DESTROYERS = ('mkfs', 'diskutil', 'fdisk', 'wipefs', 'format')
INLINE = {'python', 'python3', 'node', 'perl', 'ruby', 'php', 'deno', 'bun'}
SENDERS = {'curl', 'wget', 'nc', 'ncat', 'netcat', 'telnet', 'ssh', 'scp', 'sftp', 'rsync', 'ftp', 'socat', 'http', 'https', 'xh', 'mail', 'sendmail'}
SECRET_PATH = re.compile(r'(^|/)(\.ssh|\.aws|\.gnupg|\.kube|\.docker|\.netrc|\.npmrc|\.pypirc|\.env(\.\w+)?|id_rsa|id_ed25519|id_ecdsa|credentials|.*\.pem|.*\.key)(/|$)', re.I)
SECRET_VAR = re.compile(r'\$\{?\w*(KEY|TOKEN|SECRET|PASSW|PASSWD|PWD_|CREDENTIAL|AUTH)\w*\}?', re.I)
def expand(p, home):
    p = p.replace('$HOME', home).replace('${HOME}', home)
    if p == '~' or p.startswith('~/'): p = home + p[1:]
    return p
def resolve(p, cwd, home):
    p = expand(p, home)
    if '$' in p or '`' in p: return None  # unknown until run
    return os.path.normpath(os.path.join(cwd, p))
def facts(line, project, home=os.path.expanduser('~'), tmp=('/tmp', '/private/tmp', '/var/folders')):
    cmds, redirects = parse(line)
    pre = set()
    for rd in redirects:
        tgt = rd.lstrip('0123456789&>< |').strip()
        r = resolve(tgt, project, home) if tgt and not tgt.startswith('&') else None
        if r: pre.add(r)
    f = {'deletes': [], 'deletes_unknown': [], 'deletes_outside': [], 'deletes_outside_hard': [], 'force_push': [], 'sends': [], 'secret_refs': [], 'writes_outside': [], 'inline_code': [], 'destroys': []}
    written = set(pre)
    cwd = project
    for w in cmds:
        if not w: continue
        # Bare env or printenv prints every variable, secrets included.
        if os.path.basename(w[0]) in ('env', 'printenv') and all('=' in a or a.startswith('-') for a in w[1:]):
            f['secret_refs'].append(os.path.basename(w[0]))
        # Wrappers run the command that follows them.
        while w and os.path.basename(w[0]) in WRAPPERS:
            w = w[1:]
            while w and (w[0].startswith('-') or '=' in w[0] or w[0].replace('.', '').isdigit()):
                w = w[1:]
        if not w: continue
        name = os.path.basename(w[0])
        args = w[1:]
        if name == 'cd' and args:
            r = resolve(args[0], cwd, home); cwd = r or cwd
        targets = []
        if name in DELETERS:
            targets = [a for a in args if not a.startswith('-')]
        elif name == 'find' and ('-delete' in args or ('-exec' in args and any(os.path.basename(a) in DELETERS for a in args))):
            targets = [a for a in args[:next((i for i, a in enumerate(args) if a.startswith('-')), len(args))]] or ['.']
        elif name == 'git' and args[:1] == ['clean']:
            targets = ['.']
        elif name == 'xargs' and any(os.path.basename(a) in DELETERS for a in args):
            f['deletes_unknown'].append(' '.join(w))
        recursive = name == 'find' or name == 'git' or any(a in ('-r', '-R', '--recursive') or (a.startswith('-') and not a.startswith('--') and ('r' in a or 'R' in a)) for a in args)
        for t in targets:
            r = resolve(t, cwd, home)
            if r is None: f['deletes_unknown'].append(t); continue
            f['deletes'].append(r)
            inside = r == project or r.startswith(project + os.sep)
            if not inside and not any(r == x or r.startswith(x + os.sep) for x in tmp) and r not in written:
                f['deletes_outside'].append(r)
                glob = any(ch in t for ch in '*?[')
                if recursive or glob or r in ('/', home) or project.startswith(r + os.sep):
                    f['deletes_outside_hard'].append(r)
        if name == 'git' and 'push' in args:
            rest = args[args.index('push') + 1:]
            forced = any(a in ('-f', '--force', '--mirror') or a.startswith('--force') for a in rest) or any(a.startswith('+') for a in rest if not a.startswith('-'))
            if forced:
                refs = [a.lstrip('+') for a in rest if not a.startswith('-')][1:]
                f['force_push'].append(refs or ['(current branch)'])
        if name in INLINE and any(a in ('-c', '-e', '--eval', '-E') for a in args):
            f['inline_code'].append(' '.join(w)[:200])
        if name.startswith(DESTROYERS) or (name == 'dd' and any(a.startswith('of=/dev/') for a in args)):
            f['destroys'].append(' '.join(w)[:200])
        if name in ('cp', 'mv', 'tee', 'install') and len(args) >= 1:
            r = resolve(args[-1], cwd, home)
            if r: written.add(r)
        if name == 'git' and 'push' in args:
            rest = args[args.index('push') + 1:]
            if '--delete' in rest or '-d' in rest or any(a.startswith(':') for a in rest):
                f['force_push'].append([a.lstrip(':') for a in rest if not a.startswith('-')][1:] or ['(current branch)'])
        if name in SENDERS:
            f['sends'].append(' '.join(w)[:200])
        for a in args:
            parts = [a] + re.split(r'[=@]', a)
            if any(p and SECRET_PATH.search(expand(p, home)) for p in parts) or SECRET_VAR.search(a): f['secret_refs'].append(a)
        if name in ('env', 'printenv', 'set', 'export') and not args: f['secret_refs'].append(name)
    for rd in redirects:
        tgt = rd.lstrip('0123456789&>< |').strip()
        if tgt.startswith('&') or tgt.startswith('/dev/'): continue
        r = resolve(tgt, cwd, home)
        if r and not (r == project or r.startswith(project + os.sep)) and not any(r.startswith(x + os.sep) for x in tmp):
            f['writes_outside'].append(r)
        if SECRET_PATH.search(tgt): f['secret_refs'].append(tgt)
    return f
SHARED = {'main', 'master', 'trunk', 'develop', 'dev', 'production', 'prod', 'release', '(current branch)'}
def floor(f):
    """The hard floor: never allowed, whatever Jev says."""
    if f['deletes_outside_hard']: return 'deletes outside the project: ' + ', '.join(f['deletes_outside_hard'][:3])
    if f['destroys']: return 'erases a disk: ' + f['destroys'][0]
    for refs in f['force_push']:
        if any(r.split(':')[-1] in SHARED or r.split(':')[-1].startswith('release') for r in refs):
            return 'force-pushes a shared branch: ' + ', '.join(refs)
    if f['sends'] and f['secret_refs']: return 'sends a secret off the machine: ' + ', '.join(f['secret_refs'][:3])
    return None
def needs_judgement(f):
    return bool(f['deletes_unknown'] or f['deletes_outside'] or f['force_push'] or f['sends'] or f['writes_outside'] or f['inline_code'])
