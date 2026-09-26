package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// When a check fails because code used a name that doesn't exist, the
// model's next move is usually to search for what does exist: several LLM
// steps on an unfamiliar API. The compiler has already said which package,
// type or module it looked in, so the names that really exist there are a
// lookup code can do at once. These are facts read from the error text.

const maxHintBytes = 4000

var (
	goUndefinedQualified = regexp.MustCompile(`(?m)^(\S+\.go):\d+:\d+: undefined: (\w+)\.(\w+)`)
	goNoFieldType        = regexp.MustCompile(`\(type \*?(?:\w+\.)?(\w+) has no field or method`)
	pyModuleNoAttr       = regexp.MustCompile(`module '([\w.]+)' has no attribute '(\w+)'`)
	pyCannotImport       = regexp.MustCompile(`cannot import name '(\w+)' from '([\w.]+)'`)
	pyObjectNoAttr       = regexp.MustCompile(`'(\w+)' object has no attribute '(\w+)'`)
	jsNoExport           = regexp.MustCompile(`requested module '([^']+)' does not provide an export named '(\w+)'`)
)

// Hints reads a failed check's output and returns the names that exist
// where the errors looked, or "" when there is nothing to add.
func Hints(ctx context.Context, dir, output string) string {
	var b strings.Builder
	seen := map[string]bool{}
	add := func(title, body string) {
		if body == "" || seen[title] || b.Len() > maxHintBytes {
			return
		}
		seen[title] = true
		fmt.Fprintf(&b, "%s:\n%s\n", title, body)
	}

	for _, m := range goUndefinedQualified.FindAllStringSubmatch(output, 5) {
		file, alias := m[1], m[2]
		if pkgDir, importPath := goPackageDir(dir, file, alias); pkgDir != "" {
			add(fmt.Sprintf("package %s (%s) exports", alias, importPath), goExports(filepath.Join(dir, pkgDir)))
		}
	}
	for _, m := range goNoFieldType.FindAllStringSubmatch(output, 3) {
		add("methods of "+m[1], goMethods(ctx, dir, m[1]))
	}
	for _, m := range pyModuleNoAttr.FindAllStringSubmatch(output, 3) {
		add("module "+m[1]+" defines", pyTopLevel(dir, m[1]))
	}
	for _, m := range pyCannotImport.FindAllStringSubmatch(output, 3) {
		add("module "+m[2]+" defines", pyTopLevel(dir, m[2]))
	}
	for _, m := range pyObjectNoAttr.FindAllStringSubmatch(output, 3) {
		for _, d := range FindDefinitions(ctx, dir, m[1]) {
			add("class "+m[1]+" ("+d.Path+")", pyMembers(d.Source))
		}
	}
	for _, m := range jsNoExport.FindAllStringSubmatch(output, 3) {
		add("module "+m[1]+" exports", jsExports(dir, m[1]))
	}
	if b.Len() == 0 {
		return ""
	}
	return "[Girdle] The names those errors refer to, as they exist in the repository:\n" + strings.TrimRight(b.String(), "\n")
}

// goPackageDir finds the directory of the package imported as alias in
// file, when it belongs to this module.
func goPackageDir(dir, file, alias string) (pkgDir, importPath string) {
	module := goModule(dir)
	if module == "" {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", ""
	}
	imp := regexp.MustCompile(`(?m)^\s*(?:import\s+)?(\w+\s+)?"([^"]+)"`)
	for _, m := range imp.FindAllStringSubmatch(string(data), -1) {
		path := m[2]
		name := strings.TrimSpace(m[1])
		if name == "" {
			name = filepath.Base(path)
			// Major-version suffixes aren't the package name: goldmark/v2 is goldmark.
			if regexp.MustCompile(`^v\d+$`).MatchString(name) {
				name = filepath.Base(filepath.Dir(path))
			}
		}
		if name != alias {
			continue
		}
		if path == module {
			return ".", path
		}
		if rest, ok := strings.CutPrefix(path, module+"/"); ok {
			return rest, path
		}
	}
	return "", ""
}

func goModule(dir string) string {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

var goExported = regexp.MustCompile(`^(func (\(\w+ \*?\w+(\[[^\]]*\])?\) )?[A-Z]\w*.*|type [A-Z]\w*.*|(var|const) [A-Z]\w*.*)$`)

// goExports lists the exported declarations of the Go package in pkgDir,
// one line each, without tests.
func goExports(pkgDir string) string {
	files, _ := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	var lines []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for line := range strings.Lines(string(data)) {
			line = strings.TrimRight(line, "\n")
			if goExported.MatchString(line) {
				lines = append(lines, "  "+strings.TrimSuffix(strings.TrimSpace(line), "{"))
			}
		}
	}
	slices.Sort(lines)
	return clipHint(lines)
}

func goMethods(ctx context.Context, dir, typeName string) string {
	out, err := Search(ctx, dir, `^func \(\w+ \*?`+regexp.QuoteMeta(typeName)+`(\[[^\]]*\])?\) [A-Z]\w*`, "", "*.go", false)
	if err != nil || strings.HasPrefix(out, "no matches") {
		return ""
	}
	return out
}

var pyDef = regexp.MustCompile(`^(def \w+\(.*|class \w+.*|[A-Za-z_]\w*\s*=)`)

func pyTopLevel(dir, module string) string {
	base := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(module, ".", "/")))
	for _, candidate := range []string{base + ".py", filepath.Join(base, "__init__.py")} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		var lines []string
		for line := range strings.Lines(string(data)) {
			line = strings.TrimRight(line, "\n")
			if pyDef.MatchString(line) && !strings.HasPrefix(line, "_") && !strings.HasPrefix(line, "def _") && !strings.HasPrefix(line, "class _") {
				lines = append(lines, "  "+strings.TrimSuffix(line, ":"))
			}
		}
		return clipHint(lines)
	}
	return ""
}

func pyMembers(classSource string) string {
	var lines []string
	for line := range strings.Lines(classSource) {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "def ") && !strings.HasPrefix(t, "def _") || strings.HasPrefix(t, "def __init__") {
			lines = append(lines, "  "+strings.TrimSuffix(t, ":"))
		}
	}
	return clipHint(lines)
}

var jsExport = regexp.MustCompile(`^export\s+(default\s+)?(async\s+)?(function\*?|class|const|let|var)\s+(\w+)`)

func jsExports(dir, module string) string {
	if !strings.HasPrefix(module, ".") && !strings.HasPrefix(module, "/") {
		return "" // a package, not a file in this repository
	}
	path := module
	if u, ok := strings.CutPrefix(module, "file://"); ok {
		path = u
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var lines []string
	for line := range strings.Lines(string(data)) {
		if jsExport.MatchString(strings.TrimSpace(line)) {
			lines = append(lines, "  "+strings.TrimRight(strings.TrimSpace(line), "{ "))
		}
	}
	return clipHint(lines)
}

func clipHint(lines []string) string {
	const maxLines = 60
	if len(lines) > maxLines {
		extra := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("  … and %d more", extra))
	}
	return strings.Join(lines, "\n")
}
