package tools

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Before a shell command runs, the tripwire needs facts about it: what it
// deletes, whether it force-pushes, whether it sends data off the machine,
// and whether it touches secrets. ast-grep parses the command with the bash
// grammar, so commands inside pipelines, lists, substitutions and `bash -c`
// strings are all found, and inline Python or JavaScript is parsed with its
// own grammar. These are facts only; the tripwire decides what they mean.

// ShellFacts are what a command line does, as far as parsing can tell.
type ShellFacts struct {
	Commands [][]string `json:"-"`
	// Deletes are resolved paths the command removes. DeletesOutside are
	// those outside the project and the temp directories, and
	// DeletesOutsideHard the ones that are recursive, a glob, the home
	// directory, the root, or a parent of the project.
	Deletes            []string   `json:"deletes,omitempty"`
	DeletesUnknown     []string   `json:"deletes_unresolved,omitempty"`
	DeletesOutside     []string   `json:"deletes_outside_project,omitempty"`
	DeletesOutsideHard []string   `json:"deletes_outside_project_recursively,omitempty"`
	ForcePushes        [][]string `json:"force_pushes_or_deletes_branches,omitempty"`
	Sends              []string   `json:"sends_over_network,omitempty"`
	SecretRefs         []string   `json:"secret_references,omitempty"`
	WritesOutside      []string   `json:"writes_outside_project,omitempty"`
	InlineEffects      []string   `json:"inline_code_effects,omitempty"`
	Destroys           []string   `json:"erases_disk,omitempty"`
}

const shellRules = `id: command
language: bash
rule:
  kind: command
  has: {field: name, pattern: $NAME}
---
id: redirect
language: bash
rule:
  kind: file_redirect
  has: {field: destination, pattern: $DEST}
`

// inlineRules find calls that spawn processes, delete files or use the
// network in inline Python or JavaScript.
const inlineRules = `id: python
language: python
rule:
  any:
    - kind: call
      has:
        field: function
        regex: '^(os\.(system|popen|remove|unlink|rmdir|removedirs|exec\w*|spawn\w*|kill|rename|replace)|subprocess\.\w+|shutil\.\w+|requests\.\w+|urllib\.\w+(\.\w+)*|http\.client\.\w+|socket\.\w+|smtplib\.\w+|ftplib\.\w+|eval|exec|.*\.(unlink|rmdir|rmtree|write_text|write_bytes))$'
    - kind: call
      all:
        - has: {field: function, regex: '^__import__$'}
        - has: {field: arguments, regex: '(os|subprocess|shutil|socket|requests|urllib|http|smtplib|ftplib|ctypes)'}
    - kind: import_statement
      regex: '(subprocess|shutil|socket|requests|urllib|http|smtplib|ftplib|ctypes)'
    - kind: import_from_statement
      regex: '(subprocess|shutil|socket|requests|urllib|http|smtplib|ftplib|ctypes)'
---
id: javascript
language: javascript
rule:
  any:
    - kind: call_expression
      has:
        field: function
        regex: '^(fetch|eval|Function|.*\.(rm|rmSync|rmdir|rmdirSync|unlink|unlinkSync|writeFile|writeFileSync|exec|execSync|spawn|spawnSync|request|connect))$'
    - kind: call_expression
      all:
        - has: {field: function, regex: '^(require|import)$'}
        - has: {field: arguments, regex: '(child_process|fs|net|http|https|dgram|tls|os)'}
`

var (
	deleters   = []string{"rm", "rmdir", "unlink", "shred", "srm"}
	destroyers = []string{"mkfs", "diskutil", "fdisk", "wipefs"}
	wrappers   = []string{"sudo", "doas", "env", "nohup", "time", "nice", "ionice", "timeout", "command", "exec", "builtin", "stdbuf"}
	senders    = []string{"curl", "wget", "nc", "ncat", "netcat", "telnet", "ssh", "scp", "sftp", "rsync", "ftp", "socat", "http", "https", "xh", "mail", "sendmail"}
	shells     = []string{"bash", "sh", "zsh", "dash"}
	pythons    = []string{"python", "python3"}
	scripts    = []string{"node", "deno", "bun"}
	others     = []string{"perl", "ruby", "php"}
	sharedRefs = []string{"main", "master", "trunk", "develop", "dev", "production", "prod", "release", "(current branch)"}

	secretPath = regexp.MustCompile(`(?i)(^|/)(\.ssh|\.aws|\.gnupg|\.kube|\.docker|\.netrc|\.npmrc|\.pypirc|\.env(\.\w+)?|id_rsa|id_ed25519|id_ecdsa|credentials|[^/]*\.pem|[^/]*\.key)(/|$)`)
	secretVar  = regexp.MustCompile(`(?i)\$\{?\w*(KEY|TOKEN|SECRET|PASSW|CREDENTIAL|AUTH)\w*\}?`)
)

// ReadShell parses line and returns its facts. project is the working
// directory; relative paths resolve against it, following any cd.
func ReadShell(ctx context.Context, line, project, home string) (ShellFacts, error) {
	var f ShellFacts
	cmds, dests, err := parseShell(ctx, line, 0)
	if err != nil {
		return f, err
	}
	f.Commands = cmds
	temps := TempDirs()
	inside := func(p string) bool { return p == project || strings.HasPrefix(p, project+"/") }
	inTemp := func(p string) bool {
		for _, t := range temps {
			t = filepath.Clean(t)
			if p == t || strings.HasPrefix(p, t+"/") {
				return true
			}
		}
		return false
	}
	resolve := func(p, cwd string) string {
		p = strings.NewReplacer("${HOME}", home, "$HOME", home).Replace(p)
		if p == "~" || strings.HasPrefix(p, "~/") {
			p = home + p[1:]
		}
		if strings.ContainsAny(p, "$`") {
			return ""
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		return filepath.Clean(p)
	}
	// Files the line writes may be deleted by it too: that's cleaning up.
	written := map[string]bool{}
	cwd := project
	for _, d := range dests {
		if r := resolve(d, cwd); r != "" {
			written[r] = true
		}
	}
	for _, w := range cmds {
		if len(w) == 0 {
			continue
		}
		if b := filepath.Base(w[0]); (b == "env" || b == "printenv") && !slices.ContainsFunc(w[1:], func(a string) bool {
			return !strings.Contains(a, "=") && !strings.HasPrefix(a, "-")
		}) {
			f.SecretRefs = append(f.SecretRefs, b)
		}
		for len(w) > 0 && slices.Contains(wrappers, filepath.Base(w[0])) {
			w = w[1:]
			for len(w) > 0 && (strings.HasPrefix(w[0], "-") || strings.Contains(w[0], "=") || isNumber(w[0])) {
				w = w[1:]
			}
		}
		if len(w) == 0 {
			continue
		}
		name, args := filepath.Base(w[0]), w[1:]
		joined := strings.Join(w, " ")
		if name == "cd" && len(args) > 0 {
			if r := resolve(args[0], cwd); r != "" {
				cwd = r
			}
		}
		var targets []string
		recursive := false
		switch {
		case slices.Contains(deleters, name):
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					recursive = recursive || a == "--recursive" || !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "rR")
				} else {
					targets = append(targets, a)
				}
			}
		case name == "find" && (slices.Contains(args, "-delete") || slices.Contains(args, "-exec") && slices.ContainsFunc(args, func(a string) bool { return slices.Contains(deleters, filepath.Base(a)) })):
			recursive = true
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					break
				}
				targets = append(targets, a)
			}
			if len(targets) == 0 {
				targets = []string{"."}
			}
		case name == "git" && len(args) > 0 && args[0] == "clean":
			recursive, targets = true, []string{"."}
		case name == "xargs" && slices.ContainsFunc(args, func(a string) bool { return slices.Contains(deleters, filepath.Base(a)) }):
			f.DeletesUnknown = append(f.DeletesUnknown, joined)
		}
		for _, t := range targets {
			r := resolve(t, cwd)
			if r == "" {
				f.DeletesUnknown = append(f.DeletesUnknown, t)
				continue
			}
			f.Deletes = append(f.Deletes, r)
			if inside(r) || inTemp(r) || written[r] {
				continue
			}
			f.DeletesOutside = append(f.DeletesOutside, r)
			if recursive || strings.ContainsAny(t, "*?[") || r == "/" || r == home || strings.HasPrefix(project, r+"/") {
				f.DeletesOutsideHard = append(f.DeletesOutsideHard, r)
			}
		}
		if name == "git" && slices.Contains(args, "push") {
			rest := args[slices.Index(args, "push")+1:]
			var refs []string
			forced := false
			for _, a := range rest {
				switch {
				case a == "-f" || a == "--mirror" || a == "--delete" || a == "-d" || strings.HasPrefix(a, "--force"):
					forced = true
				case strings.HasPrefix(a, "-"):
				default:
					if strings.HasPrefix(a, "+") || strings.HasPrefix(a, ":") {
						forced = true
					}
					refs = append(refs, strings.TrimLeft(a, "+:"))
				}
			}
			if forced {
				if len(refs) > 0 {
					refs = refs[1:] // the first is the remote
				}
				if len(refs) == 0 {
					refs = []string{"(current branch)"}
				}
				f.ForcePushes = append(f.ForcePushes, refs)
			}
		}
		switch {
		case slices.Contains(pythons, name) && slices.Contains(args, "-c"):
			f.InlineEffects = append(f.InlineEffects, inlineEffects(ctx, "python", argAfter(args, "-c"))...)
		case slices.Contains(scripts, name) && (slices.Contains(args, "-e") || slices.Contains(args, "--eval")):
			f.InlineEffects = append(f.InlineEffects, inlineEffects(ctx, "javascript", cmp.Or(argAfter(args, "-e"), argAfter(args, "--eval")))...)
		case slices.Contains(others, name) && slices.ContainsFunc(args, func(a string) bool { return a == "-e" || a == "-r" || a == "-E" }):
			f.InlineEffects = append(f.InlineEffects, name+" inline code")
		}
		if slices.ContainsFunc(destroyers, func(d string) bool { return strings.HasPrefix(name, d) }) || name == "dd" && slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "of=/dev/") }) {
			f.Destroys = append(f.Destroys, joined)
		}
		if slices.Contains(senders, name) {
			f.Sends = append(f.Sends, clipTail(joined, 200))
		}
		for _, a := range args {
			if isSecret(resolveHome(a, home)) || secretVar.MatchString(a) {
				f.SecretRefs = append(f.SecretRefs, a)
			}
		}
	}
	for _, d := range dests {
		if strings.HasPrefix(d, "/dev/") || isNumber(d) {
			continue
		}
		// A file the same line writes and then deletes leaves nothing behind.
		if r := resolve(d, cwd); r != "" && !inside(r) && !inTemp(r) && !slices.Contains(f.Deletes, r) {
			f.WritesOutside = append(f.WritesOutside, r)
		}
		if isSecret(d) {
			f.SecretRefs = append(f.SecretRefs, d)
		}
	}
	return f, nil
}

// TempDirs are the system's temporary directories: deleting inside them
// isn't deleting the user's files.
func TempDirs() []string {
	return []string{filepath.Clean(os.TempDir()), "/tmp", "/private/tmp", "/var/folders", "/private/var/folders"}
}

// Floor is the hard floor under the tripwire: what is never run, whatever
// Jev says, because injected text can steer Jev. It returns why, or "".
func (f ShellFacts) Floor() string {
	switch {
	case len(f.DeletesOutsideHard) > 0:
		return "it deletes outside the project: " + strings.Join(first(f.DeletesOutsideHard, 3), ", ")
	case len(f.Destroys) > 0:
		return "it erases a disk: " + f.Destroys[0]
	case len(f.Sends) > 0 && len(f.SecretRefs) > 0:
		return "it sends a secret off the machine: " + strings.Join(first(f.SecretRefs, 3), ", ")
	}
	for _, refs := range f.ForcePushes {
		for _, r := range refs {
			_, branch, _ := strings.Cut(r, ":")
			branch = cmp.Or(branch, r)
			if slices.Contains(sharedRefs, branch) || strings.HasPrefix(branch, "release") {
				return "it force-pushes or deletes a shared branch: " + strings.Join(refs, ", ")
			}
		}
	}
	return ""
}

// NeedsJudgement reports whether the command does anything the tripwire
// should ask Jev about. Most commands don't, and run at once.
func (f ShellFacts) NeedsJudgement() bool {
	return len(f.DeletesUnknown)+len(f.DeletesOutside)+len(f.ForcePushes)+len(f.Sends)+len(f.WritesOutside)+len(f.InlineEffects) > 0
}

// parseShell returns the words of every command in line, and the
// destinations of its file redirects. `bash -c` strings and eval are
// parsed too.
func parseShell(ctx context.Context, line string, depth int) (cmds [][]string, dests []string, err error) {
	cmd := exec.CommandContext(ctx, "ast-grep", "scan", "--inline-rules", shellRules, "--json=stream", "--stdin")
	cmd.Stdin = strings.NewReader(line)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var m struct {
			RuleID        string `json:"ruleId"`
			Text          string `json:"text"`
			MetaVariables struct {
				Single map[string]struct {
					Text string `json:"text"`
				} `json:"single"`
			} `json:"metaVariables"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m.RuleID == "redirect" {
			if d := shellWords(m.MetaVariables.Single["DEST"].Text); len(d) > 0 {
				dests = append(dests, d[0])
			}
			continue
		}
		w := shellWords(m.Text)
		cmds = append(cmds, w)
		if depth < 3 && len(w) > 1 {
			var inner string
			switch {
			case slices.Contains(shells, filepath.Base(w[0])) && slices.Contains(w, "-c"):
				inner = argAfter(w, "-c")
			case w[0] == "eval":
				inner = strings.Join(w[1:], " ")
			}
			if inner != "" {
				c2, d2, _ := parseShell(ctx, inner, depth+1)
				cmds, dests = append(cmds, c2...), append(dests, d2...)
			}
		}
	}
	return cmds, dests, nil
}

// inlineEffects parses inline code and returns the calls in it that spawn
// processes, delete files or use the network.
func inlineEffects(ctx context.Context, lang, code string) []string {
	if code == "" {
		return nil
	}
	// Only the rule for the code's own language runs.
	cmd := exec.CommandContext(ctx, "ast-grep", "scan", "--inline-rules", inlineRules, "--filter", "^"+lang+"$", "--json=stream", "--stdin")
	cmd.Stdin = strings.NewReader(code)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		// Code that can't be checked is judged.
		return []string{lang + " inline code that couldn't be parsed"}
	}
	var effects []string
	for l := range strings.Lines(string(out)) {
		var m struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(l), &m) == nil && m.Text != "" {
			effects = append(effects, clipTail(m.Text, 120))
		}
	}
	return effects
}

// shellWords splits a command's text into words, honouring quotes and
// backslash escapes.
func shellWords(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	started := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote != 0:
			switch {
			case c == quote:
				quote = 0
			case c == '\\' && quote == '"' && i+1 < len(rs):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(c)
			}
		case c == '"' || c == '\'':
			quote, started = c, true
		case c == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			started = true
		case c == ' ' || c == '\t' || c == '\n':
			if started || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(c)
		}
	}
	if started || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func argAfter(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func isSecret(a string) bool {
	for _, part := range append([]string{a}, strings.FieldsFunc(a, func(r rune) bool { return r == '=' || r == '@' })...) {
		if part != "" && secretPath.MatchString(part) {
			return true
		}
	}
	return false
}

func resolveHome(p, home string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return home + p[1:]
	}
	return p
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return true
}

func first(s []string, n int) []string { return s[:min(len(s), n)] }
