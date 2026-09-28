package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/fantasy"
)

// maxLookupBytes caps one lookup's output: enough for several files and
// searches, small enough to keep the context usable.
const maxLookupBytes = 120_000

// LookupDescription is the lookup tool's description.
const LookupDescription = "Fetch everything you need to see in one call: whole files or line ranges, the source of named functions, methods, classes or types, and regular-expression searches. Each LLM response costs seconds, so ask for all of it at once rather than one thing at a time."

// LookupInput is the lookup tool's input. Every list is optional.
type LookupInput struct {
	Files       []string `json:"files,omitempty" description:"Files to read, relative to the working directory. Add :START-END for a line range, for example parser/parser.go:1200-1320. Whole files stop at 2000 lines."`
	Definitions []string `json:"definitions,omitempty" description:"Names of functions, methods, classes or types (Go, Python, JavaScript, TypeScript) whose source to show, for example parseHeading."`
	Searches    []string `json:"searches,omitempty" description:"Regular expressions to search every file for, in ripgrep syntax. Each returns at most 80 matching lines."`
}

func (t toolset) lookup(ctx context.Context, in LookupInput, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if len(in.Files)+len(in.Definitions)+len(in.Searches) == 0 {
		return fantasy.NewTextErrorResponse("nothing to look up: give files, definitions or searches"), nil
	}
	var b strings.Builder
	section := func(title, body string) {
		if b.Len() > maxLookupBytes {
			return
		}
		fmt.Fprintf(&b, "=== %s\n%s\n\n", title, strings.TrimRight(body, "\n"))
		if b.Len() > maxLookupBytes {
			fmt.Fprintf(&b, "[lookup output cut at %d KB; ask for less at once]\n", maxLookupBytes>>10)
		}
	}
	for _, f := range in.Files {
		path, offset, limit := parseRange(f)
		res, _ := t.read(ctx, readInput{Path: path, Offset: offset, Limit: limit}, call)
		title := "file " + f
		if res.IsError {
			title += " (error)"
		}
		section(title, res.Content)
	}
	var files []string
	if len(in.Definitions) > 0 {
		files = SourceFiles(ctx, t.dir)
	}
	for _, name := range in.Definitions {
		defs := FindDefinitions(t.dir, files, name)
		if len(defs) == 0 {
			section("definition "+name, "not found; try a search")
			continue
		}
		var parts []string
		for _, d := range defs {
			parts = append(parts, d.String())
		}
		section("definition "+name, strings.Join(parts, "\n\n"))
	}
	for _, pattern := range in.Searches {
		out, err := Search(ctx, t.dir, pattern, "", false)
		switch {
		case err != nil:
			out = err.Error()
		case t.grepCtx:
			// A search that finds nothing costs a step to retry; the
			// commonest near miss is the case of a name.
			if strings.HasPrefix(out, "no matches") {
				if alt, err := Search(ctx, t.dir, pattern, "", true); err == nil && !strings.HasPrefix(alt, "no matches") {
					out = "no matches with this case; these match ignoring case:\n" + alt
				}
			}
			if note := t.aroundMatches(out); note != "" {
				out += "\n\n" + note
			}
		}
		section("search "+pattern, out)
	}
	return fantasy.NewTextResponse(strings.TrimRight(b.String(), "\n")), nil
}

// parseRange splits "path:START-END" into a path, a 1-based offset and a
// line count. A plain path reads from the start.
func parseRange(s string) (path string, offset, limit int) {
	path, rng, ok := strings.Cut(s, ":")
	if !ok {
		return s, 0, 0
	}
	from, to, _ := strings.Cut(rng, "-")
	start, err1 := strconv.Atoi(strings.TrimSpace(from))
	end, err2 := strconv.Atoi(strings.TrimSpace(to))
	switch {
	case err1 != nil:
		return s, 0, 0 // a colon that isn't a range, say a Windows path
	case err2 != nil || end < start:
		return path, start, 0
	default:
		return path, start, end - start + 1
	}
}
