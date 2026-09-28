package tools

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"charm.land/fantasy"
)

// BatchedWithReset returns the fast flow's tools, lookup, apply and bash, and
// a function the kernel calls at the start of each request, so apply's
// reproduce knows what the request has changed. Each LLM step costs seconds
// of latency, so both working tools take lists: lookup fetches every file,
// definition and search the model needs in one call, and apply makes every
// change and runs the check that verifies them.
func BatchedWithReset(dir string, reproduce bool, opts Options) ([]fantasy.AgentTool, func()) {
	t := newToolset(dir, opts)
	reset := func() {}
	if reproduce {
		t.orig = &originals{files: map[string]*[]byte{}}
		reset = t.orig.reset
	}
	return []fantasy.AgentTool{
		fantasy.NewAgentTool("lookup", LookupDescription, t.lookup),
		fantasy.NewAgentTool("apply", ApplyDescription, t.apply),
		fantasy.NewAgentTool("bash", "Run a shell command in the working directory and return its combined output and exit code. Use lookup, not bash, to read or search files.", t.bash),
	}, reset
}

// ApplyDescription is the apply tool's description, shared by every tool that
// takes an ApplyInput.
const ApplyDescription = "Apply file changes in order, then run a check command. Each change either replaces exact text in a file (old_text must match exactly once) or, without old_text, writes the whole file. The check runs only if every change applied, with pipefail set, and its output ends with the exit code."

// Change is one edit or whole-file write inside an apply call.
type Change struct {
	Path       string `json:"path" description:"File path, relative to the working directory or absolute."`
	OldText    string `json:"old_text,omitempty" description:"Exact text to replace, including whitespace. Leave it out to write the whole file from content."`
	NewText    string `json:"new_text,omitempty" description:"Replacement for old_text. Leave it out to delete old_text."`
	ReplaceAll bool   `json:"replace_all,omitempty" description:"Replace every occurrence of old_text, for example to rename something everywhere in the file, comments included."`
	Content    string `json:"content,omitempty" description:"The whole new file, used when old_text is left out."`
}

// ApplyInput is the apply tool's input.
type ApplyInput struct {
	Changes Changes `json:"changes" description:"Every edit and file write the task needs, applied in order. Leave it empty to only run the check."`
	Check   string  `json:"check" description:"Shell command that proves the task is done: build and run the tests, plus quick checks for anything the tests can't show, for example grep that an old name is gone. Don't hide its exit code."`
	// Reproduce is optional: see its description.
	Reproduce string `json:"reproduce,omitempty" description:"For a bug fix: a command that runs only the test reproducing the bug. After a passing check, Girdle runs it once with your code changes undone, to show it fails without the fix."`
}

// Changes is the list of changes in an apply call. Models sometimes send it
// as a string holding the JSON array; that is accepted too, since rejecting
// it makes the model write every file again.
type Changes []Change

// UnmarshalJSON accepts a JSON array of changes, or a string containing one.
func (c *Changes) UnmarshalJSON(data []byte) error {
	var list []Change
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		data = []byte(s)
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*c = list
	return nil
}

func (t toolset) apply(ctx context.Context, in ApplyInput, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var b strings.Builder
	failed := 0
	for _, c := range in.Changes {
		if t.orig != nil && c.Path != "" {
			t.orig.remember(t.path(c.Path))
		}
		var res fantasy.ToolResponse
		if c.OldText != "" {
			res, _ = t.edit(ctx, editInput{Path: c.Path, OldText: c.OldText, NewText: c.NewText, ReplaceAll: c.ReplaceAll}, call)
		} else {
			res, _ = t.write(ctx, writeInput{Path: c.Path, Content: c.Content}, call)
		}
		mark := "✓"
		if res.IsError {
			mark = "✗"
			failed++
		}
		fmt.Fprintf(&b, "%s %s\n", mark, res.Content)
	}
	switch {
	case len(in.Changes) == 0:
		b.WriteString("no changes given\n")
	case failed > 0:
		fmt.Fprintf(&b, "%d of %d changes failed; the others were applied. Check not run: fix the failed changes first.", failed, len(in.Changes))
		return fantasy.NewTextErrorResponse(b.String()), nil
	}
	if in.Check == "" {
		b.WriteString("no check given")
		return fantasy.NewTextResponse(b.String()), nil
	}
	fmt.Fprintf(&b, "$ %s\n", in.Check)
	run := t.check(ctx, in.Check)
	// A test that doesn't reproduce the bug turns the result into a failure,
	// so it isn't taken as proof.
	if run.code == 0 && in.Reproduce != "" {
		if note, weak := t.reproduce(ctx, in.Reproduce); note != "" {
			run.note(note)
			if weak {
				run.code = 1
			}
		}
	}
	if run.code != 0 {
		if hints := Hints(ctx, t.dir, run.out); hints != "" {
			run.note(hints)
		}
	}
	b.WriteString(run.text())
	return fantasy.NewTextResponse(b.String()), nil
}

// ParseApplyInput decodes an apply call's input.
func ParseApplyInput(input string) (ApplyInput, bool) {
	var in ApplyInput
	return in, json.Unmarshal([]byte(input), &in) == nil
}

// Paths lists the files the changes touch, once each, in order.
func (in ApplyInput) Paths() []string {
	var paths []string
	for _, c := range in.Changes {
		if !slices.Contains(paths, c.Path) {
			paths = append(paths, c.Path)
		}
	}
	return paths
}

// RunCheck runs a check command in dir the way apply does, and returns its
// output, ending with the exit-code line, and its exit code. ran is false
// when the tripwire or the platform refused the command, so its exit code
// says nothing about the code under test.
func RunCheck(ctx context.Context, dir, command string, opts Options) (out string, code int, ran bool) {
	r := newToolset(dir, opts).check(ctx, command)
	return r.text(), r.code, !r.refused
}

// check runs a check command with pipefail, so that "go test | tail" still
// fails when the tests do.
func (t toolset) check(ctx context.Context, command string) shellRun {
	return t.shell(ctx, "set -o pipefail\n"+command, defaultTimeout)
}
