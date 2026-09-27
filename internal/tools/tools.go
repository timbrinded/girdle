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

// All returns the built-in tools bound to dir. guard, if set, sees every
// shell command first.
func All(dir string, guard Guard) []fantasy.AgentTool {
	t := toolset{dir: dir, guard: guard}
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
	if t.guard != nil {
		if why := t.guard(ctx, in.Command); why != "" {
			return fantasy.NewTextErrorResponse("[Girdle] Not run: " + why + ". Girdle never runs a command that deletes outside the project, force-pushes a shared branch, or sends secrets off the machine. If the task needs it, stop and tell the user.\n" + exitTrailer + "126]"), nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
	cmd.Dir = t.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()

	code := 0
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = -1
	}
	text := clip(out.String())
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		text += fmt.Sprintf("\n[timed out after %s]", timeout)
	case err != nil && code == -1:
		text += "\n[" + err.Error() + "]"
	}
	return fantasy.NewTextResponse(fmt.Sprintf("%s\n%s%d]", strings.TrimRight(text, "\n"), exitTrailer, code)), nil
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
