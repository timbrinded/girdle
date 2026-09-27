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

// Units are the top-level pieces of a source file as its parser sees them:
// a function, method, type, class, or top-level statement, with the comment
// block above it. ast-grep does the parsing, so a unit's bounds are exact
// rather than guessed from indentation or braces.

// Unit is one top-level piece of a source file.
type Unit struct {
	Path       string
	Start, End int // 1-based, inclusive; Start includes the comment above
	Name       string
	Text       string
}

// Lines is the unit's size in lines.
func (u Unit) Lines() int { return u.End - u.Start + 1 }

// unitRules holds one ast-grep rule per language: the node kinds that sit
// directly under the file's root.
const unitRules = `id: go
language: go
rule:
  any: [{kind: function_declaration}, {kind: method_declaration}, {kind: type_declaration}, {kind: var_declaration}, {kind: const_declaration}]
  inside: {kind: source_file, stopBy: neighbor}
---
id: python
language: python
rule:
  any: [{kind: function_definition}, {kind: class_definition}, {kind: decorated_definition}, {kind: expression_statement}]
  inside: {kind: module, stopBy: neighbor}
---
id: javascript
language: javascript
rule:
  any: [{kind: function_declaration}, {kind: class_declaration}, {kind: lexical_declaration}, {kind: export_statement}, {kind: expression_statement}]
  inside: {kind: program, stopBy: neighbor}
---
id: typescript
language: typescript
rule:
  any: [{kind: function_declaration}, {kind: class_declaration}, {kind: lexical_declaration}, {kind: export_statement}, {kind: expression_statement}, {kind: interface_declaration}, {kind: type_alias_declaration}]
  inside: {kind: program, stopBy: neighbor}
`

// HaveAstGrep reports whether the ast-grep binary is installed.
func HaveAstGrep() bool {
	_, err := exec.LookPath("ast-grep")
	return err == nil
}

var unitName = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:def|func|class|type|function|interface|const|let|var)\s+(?:\([^)]*\)\s*)?([A-Za-z_$][A-Za-z0-9_$]*)`)

// Units parses the source files under dir and returns their top-level
// units, or nil when ast-grep isn't installed.
func Units(ctx context.Context, dir string) []Unit {
	if !HaveAstGrep() {
		return nil
	}
	cmd := exec.CommandContext(ctx, "ast-grep", "scan", "--inline-rules", unitRules, "--json=stream", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	var units []Unit
	files := map[string][]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var m struct {
			File  string `json:"file"`
			Text  string `json:"text"`
			Range struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
				End struct {
					Line int `json:"line"`
				} `json:"end"`
			} `json:"range"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		p := filepath.ToSlash(strings.TrimPrefix(m.File, "./"))
		u := Unit{Path: p, Start: m.Range.Start.Line + 1, End: m.Range.End.Line + 1, Text: m.Text}
		if n := unitName.FindStringSubmatch(m.Text); n != nil {
			u.Name = n[1]
		}
		lines, ok := files[p]
		if !ok {
			if data, err := os.ReadFile(filepath.Join(dir, p)); err == nil {
				lines = strings.Split(string(data), "\n")
			}
			files[p] = lines
		}
		// Take in the comment block directly above: it is often the spec.
		for u.Start > 1 && u.Start-2 < len(lines) && isCommentLine(lines[u.Start-2]) {
			u.Start--
			u.Text = lines[u.Start-1] + "\n" + u.Text
		}
		units = append(units, u)
	}
	// ast-grep reads files in parallel; a fixed order keeps snapshots, and
	// so prompt caches, stable.
	slices.SortFunc(units, func(a, b Unit) int { return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Start, b.Start)) })
	return units
}

func isCommentLine(l string) bool {
	l = strings.TrimSpace(l)
	return strings.HasPrefix(l, "//") || strings.HasPrefix(l, "/*") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "#")
}

// identifierWord matches identifiers: what a unit refers to by name.
var identifierWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// Referenced returns the names text mentions, once each.
func Referenced(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range identifierWord.FindAllString(text, -1) {
		out[w] = true
	}
	return out
}

// Mentions lists where name appears as a whole word, comments and strings
// included, as "path:line: text", at most limit of them.
func Mentions(ctx context.Context, dir, name string, limit int) []string {
	if !identifier.MatchString(name) {
		return nil
	}
	out, err := Search(ctx, dir, `\b`+name+`\b`, "", "", false)
	if err != nil || strings.HasPrefix(out, "no matches") {
		return nil
	}
	var lines []string
	for l := range strings.Lines(out) {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "…") {
			lines = append(lines, l)
		}
	}
	return lines[:min(len(lines), limit)]
}

var sgLanguage = map[string]string{".go": "go", ".py": "python", ".js": "javascript", ".mjs": "javascript", ".ts": "typescript"}

// UnusedDefs lists the functions defined in file that no code in the
// repository refers to. Mentions in comments and strings don't count: the
// parser tells code from prose, which a text search can't.
func UnusedDefs(ctx context.Context, dir, file string) []string {
	lang := sgLanguage[filepath.Ext(file)]
	if lang == "" || !HaveAstGrep() {
		return nil
	}
	cmd := exec.CommandContext(ctx, "ast-grep", "scan", "--inline-rules", unitRules, "--filter", "^"+lang+"$", "--json=stream", file)
	cmd.Dir = dir
	out, _ := cmd.Output()
	type def struct {
		name string
		line int
	}
	var defs []def
	for l := range strings.Lines(string(out)) {
		var m struct {
			Text  string `json:"text"`
			Range struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"range"`
		}
		if json.Unmarshal([]byte(l), &m) != nil {
			continue
		}
		n := unitName.FindStringSubmatchIndex(m.Text)
		if n == nil {
			continue
		}
		if kw := m.Text[n[0]:n[3]]; strings.Contains(kw, "def ") || strings.Contains(kw, "func ") || strings.Contains(kw, "function ") {
			// The name's own line is where the parser saw the definition.
			defs = append(defs, def{m.Text[n[2]:n[3]], m.Range.Start.Line + strings.Count(m.Text[:n[2]], "\n")})
		}
	}
	var unused []string
	for _, d := range defs {
		cmd := exec.CommandContext(ctx, "ast-grep", "run", "-l", lang, "-p", d.name, "--json=stream", ".")
		cmd.Dir = dir
		out, _ := cmd.Output()
		used := false
		for l := range strings.Lines(string(out)) {
			var m struct {
				File  string `json:"file"`
				Range struct {
					Start struct {
						Line int `json:"line"`
					} `json:"start"`
				} `json:"range"`
			}
			if json.Unmarshal([]byte(l), &m) == nil && !(filepath.Clean(m.File) == filepath.Clean(file) && m.Range.Start.Line == d.line) {
				used = true
				break
			}
		}
		if !used {
			unused = append(unused, d.name)
		}
	}
	return unused
}
