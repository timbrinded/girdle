// Package tools provides Girdle's four built-in tools: read, write, edit and
// bash. Paths are resolved against the session's working directory.
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
)

const (
	maxReadLines   = 2000
	maxOutputBytes = 20_000
	defaultTimeout = 120 * time.Second
	maxTimeout     = 600 * time.Second
)

// Guard is asked before any shell command runs. It returns why the
// command must not run, or "" to let it run.
type Guard func(ctx context.Context, command string) string

// Options shape how the tools run shell commands.
type Options struct {
	// Guard, if set, sees every shell command first.
	Guard Guard
	// Offline runs shell commands without access to other machines;
	// connections to this machine still work. A benchmark uses it so an
	// agent can't fetch the upstream fix it is being tested on.
	Offline bool
	// DenyRead lists regular expressions for absolute paths the tools may
	// not read, other than inside the working directory: other copies of
	// the code under test, which a benchmark agent could copy the fix from.
	// Keep them simple (anchors, escapes, bracket sets): shell commands get
	// them from macOS's sandbox-exec, whose regex dialect is POSIX's.
	DenyRead []*regexp.Regexp
}

// offlineProfile is the macOS sandbox profile for offline commands.
const offlineProfile = `(version 1)(allow default)(deny network-outbound (remote ip))(allow network-outbound (remote ip "localhost:*"))`

// sandboxProfile is the macOS sandbox profile for shell commands: offline if
// asked, and unable to read denied paths except inside dir.
// Later rules win, so dir's allow comes last.
func sandboxProfile(dir string, offline bool, deny []*regexp.Regexp) string {
	var b strings.Builder
	b.WriteString("(version 1)(allow default)")
	if offline {
		b.WriteString(strings.TrimPrefix(offlineProfile, "(version 1)(allow default)"))
	}
	if len(deny) > 0 {
		for _, re := range deny {
			fmt.Fprintf(&b, `(deny file-read* (regex #"%s"))`, re.String())
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		fmt.Fprintf(&b, `(allow file-read* (subpath %q))`, dir)
	}
	return b.String()
}

// denied reports whether p, an absolute path, matches a denied pattern and
// is outside the working directory.
func (t toolset) denied(p string) bool {
	if within(p, t.dir) {
		return false
	}
	for _, re := range t.denyRead {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// deniedError is what a tool says when asked to touch a denied path.
func deniedError(p string) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(p + " can't be read here: it is outside the working directory, in a place this session may not read")
}

// All returns the built-in tools bound to dir.
func All(dir string, opts Options) []fantasy.AgentTool {
	t := newToolset(dir, opts)
	return []fantasy.AgentTool{
		fantasy.NewAgentTool("read", "Read a text file. Returns up to 2000 lines starting at offset (1-based).", t.read),
		fantasy.NewAgentTool("write", "Create or overwrite a file with the given content. Creates parent directories.", t.write),
		fantasy.NewAgentTool("edit", "Replace exact text in a file. old_text must match exactly once unless replace_all is true.", t.edit),
		fantasy.NewAgentTool("bash", "Run a shell command in the working directory and return its combined output and exit code.", t.bash),
	}
}

// Tool inputs mark optional fields with omitempty: Fantasy reads that tag,
// not omitzero, when it decides which parameters the model must send.

type toolset struct {
	dir string
	// orig, when set, remembers files' contents from before the current
	// request changed them, for apply's reproduce.
	orig *originals
	// guard, when set, sees every shell command before it runs.
	guard Guard
	// offline runs shell commands without outside network access.
	offline bool
	// denyRead matches the paths the tools may not read outside dir.
	denyRead []*regexp.Regexp
}

func newToolset(dir string, opts Options) toolset {
	return toolset{dir: dir, guard: opts.Guard, offline: opts.Offline, denyRead: opts.DenyRead}
}

func (t toolset) path(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(t.dir, p)
}

type readInput struct {
	Path   string `json:"path" description:"File path, relative to the working directory or absolute."`
	Offset int    `json:"offset,omitempty" description:"First line to return, 1-based. Defaults to 1."`
	Limit  int    `json:"limit,omitempty" description:"Maximum lines to return. Defaults to 2000."`
}

func (t toolset) read(_ context.Context, in readInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if p := t.path(in.Path); t.denied(p) {
		return deniedError(p), nil
	}
	data, err := os.ReadFile(t.path(in.Path))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	lines := strings.Split(string(data), "\n")
	start := max(in.Offset, 1) - 1
	if start >= len(lines) {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("offset %d is past the end of the file (%d lines)", in.Offset, len(lines))), nil
	}
	limit := in.Limit
	if limit <= 0 || limit > maxReadLines {
		limit = maxReadLines
	}
	end := min(start+limit, len(lines))
	out := strings.ToValidUTF8(strings.Join(lines[start:end], "\n"), "\uFFFD")
	if end < len(lines) {
		out += fmt.Sprintf("\n[showing lines %d-%d of %d; use offset to read more]", start+1, end, len(lines))
	}
	return fantasy.NewTextResponse(out), nil
}

type writeInput struct {
	Path    string `json:"path" description:"File path, relative to the working directory or absolute."`
	Content string `json:"content" description:"The full file content."`
}

func (t toolset) write(_ context.Context, in writeInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	p := t.path(in.Path)
	if t.denied(p) {
		return deniedError(p), nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if err := os.WriteFile(p, []byte(in.Content), 0o644); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.NewTextResponse(fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path)), nil
}

type editInput struct {
	Path       string `json:"path" description:"File path, relative to the working directory or absolute."`
	OldText    string `json:"old_text" description:"Exact text to replace, including whitespace."`
	NewText    string `json:"new_text" description:"Replacement text."`
	ReplaceAll bool   `json:"replace_all,omitempty" description:"Replace every occurrence instead of exactly one."`
}

func (t toolset) edit(_ context.Context, in editInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	p := t.path(in.Path)
	if t.denied(p) {
		return deniedError(p), nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	s := string(data)
	n := strings.Count(s, in.OldText)
	switch {
	case in.OldText == "":
		return fantasy.NewTextErrorResponse("old_text is empty"), nil
	case n == 0:
		// Models often get indentation wrong, tabs for spaces or the
		// reverse. A unique match with whitespace ignored is safe to use.
		if out, ok := replaceIgnoringWhitespace(s, in.OldText, in.NewText); ok {
			if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(fmt.Sprintf("replaced 1 occurrence(s) in %s (old_text matched with whitespace ignored; indentation kept from the file)", in.Path)), nil
		}
		return fantasy.NewTextErrorResponse("old_text not found in " + in.Path), nil
	case n > 1 && !in.ReplaceAll:
		return fantasy.NewTextErrorResponse(fmt.Sprintf("old_text matches %d times in %s; add surrounding lines to make it unique or set replace_all", n, in.Path)), nil
	}
	replaced := 1
	if in.ReplaceAll {
		replaced = n
	}
	s = strings.Replace(s, in.OldText, in.NewText, replaced)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.NewTextResponse(fmt.Sprintf("replaced %d occurrence(s) in %s", replaced, in.Path)), nil
}

type bashInput struct {
	Command        string `json:"command" description:"The shell command to run."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" description:"Timeout in seconds. Defaults to 120, maximum 600."`
}

func (t toolset) bash(ctx context.Context, in bashInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	timeout := defaultTimeout
	if in.TimeoutSeconds > 0 {
		timeout = min(time.Duration(in.TimeoutSeconds)*time.Second, maxTimeout)
	}
	r := t.shell(ctx, in.Command, timeout)
	if r.refused {
		return fantasy.NewTextErrorResponse(r.text()), nil
	}
	return fantasy.NewTextResponse(r.text()), nil
}

// shellRun is one shell command's result.
type shellRun struct {
	out     string // combined output, clipped, with any note about how it ended
	code    int
	refused bool // the guard or the platform refused to run it
}

// text is the result as the tools report it: the output, then the exit code
// on its own line.
func (r shellRun) text() string {
	return fmt.Sprintf("%s\n%s%d]", strings.TrimRight(r.out, "\n"), exitTrailer, r.code)
}

func (t toolset) shell(ctx context.Context, command string, timeout time.Duration) shellRun {
	if t.guard != nil {
		if why := t.guard(ctx, command); why != "" {
			return shellRun{out: "[Girdle] Not run: " + why + ". Girdle never runs a command that deletes outside the project, force-pushes a shared branch, or sends secrets off the machine. If the task needs it, stop and tell the user.", code: 126, refused: true}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name, args := "bash", []string{"-c", command}
	if t.offline || len(t.denyRead) > 0 {
		if runtime.GOOS != "darwin" {
			return shellRun{out: "[Girdle] Sandboxed commands need macOS's sandbox-exec, so this command was not run.", code: 126, refused: true}
		}
		name, args = "sandbox-exec", append([]string{"-p", sandboxProfile(t.dir, t.offline, t.denyRead), "bash"}, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = t.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var r shellRun

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		r.code = exitErr.ExitCode()
	} else if err != nil {
		r.code = -1
	}
	r.out = clip(out.String())
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		r.out += fmt.Sprintf("\n[timed out after %s]", timeout)
	case err != nil && r.code == -1:
		r.out += "\n[" + err.Error() + "]"
	}
	return r
}

const exitTrailer = "[exit code "

// ExitCode reads the exit code that the bash tool appends to its output.
func ExitCode(result string) (int, bool) {
	_, after, found := strings.CutLast(result, exitTrailer)
	if !found {
		return 0, false
	}
	num, ok := strings.CutSuffix(strings.TrimSpace(after), "]")
	if !ok {
		return 0, false
	}
	code, err := strconv.Atoi(num)
	return code, err == nil
}

// clip keeps the head and tail of long output, where errors usually are. It
// returns valid UTF-8, cutting only on character boundaries.
func clip(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= maxOutputBytes {
		return s
	}
	head := maxOutputBytes / 4
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	tail := len(s) - maxOutputBytes*3/4
	for tail < len(s) && !utf8.RuneStart(s[tail]) {
		tail++
	}
	return s[:head] + fmt.Sprintf("\n[... %d bytes omitted ...]\n", tail-head) + s[tail:]
}

// replaceIgnoringWhitespace replaces old with new in content, matching whole
// lines with leading and trailing whitespace ignored. It succeeds only when
// exactly one run of lines matches. The new lines are re-indented the way
// the matched lines differ from old: the same prefix mapping, and runs of
// spaces turned into tabs when the file indents with tabs.
func replaceIgnoringWhitespace(content, old, new string) (string, bool) {
	oldLines := strings.Split(strings.Trim(old, "\n"), "\n")
	fileLines := strings.Split(content, "\n")
	if len(oldLines) == 0 || strings.TrimSpace(old) == "" {
		return "", false
	}
	match := -1
	for i := 0; i+len(oldLines) <= len(fileLines); i++ {
		same := true
		for j, ol := range oldLines {
			if strings.TrimSpace(fileLines[i+j]) != strings.TrimSpace(ol) {
				same = false
				break
			}
		}
		if same {
			if match >= 0 {
				return "", false // ambiguous
			}
			match = i
		}
	}
	if match < 0 {
		return "", false
	}

	indents := map[string]string{}
	spacesPerTab := 0
	for j, ol := range oldLines {
		if strings.TrimSpace(ol) == "" {
			continue
		}
		from, to := leading(ol), leading(fileLines[match+j])
		indents[from] = to
		if from != "" && strings.Trim(from, " ") == "" && to != "" && strings.Trim(to, "\t") == "" && len(from)%len(to) == 0 {
			spacesPerTab = len(from) / len(to)
		}
	}
	newLines := strings.Split(strings.Trim(new, "\n"), "\n")
	if strings.Trim(new, "\n") == "" {
		newLines = nil
	}
	for k, nl := range newLines {
		lead := leading(nl)
		switch to, ok := indents[lead]; {
		case ok:
			newLines[k] = to + nl[len(lead):]
		case spacesPerTab > 0 && lead != "" && strings.Trim(lead, " ") == "" && len(lead)%spacesPerTab == 0:
			newLines[k] = strings.Repeat("\t", len(lead)/spacesPerTab) + nl[len(lead):]
		}
	}
	out := slices.Concat(fileLines[:match], newLines, fileLines[match+len(oldLines):])
	return strings.Join(out, "\n"), true
}

func leading(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}
