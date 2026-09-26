package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTakeSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.go":               "package main\n\nfunc main() {}\n",
		"sub/util.go":           "package sub\n",
		"logo.png":              "\x89PNG\x00\x00binary",
		"big.txt":               strings.Repeat("x", 500),
		"node_modules/dep/a.js": "ignored",
		".git/config":           "ignored",
	})
	snap := TakeSnapshot(t.Context(), dir, 100)

	if snap.Files != 4 || snap.Included != 2 {
		t.Fatalf("Files=%d Included=%d, want 4 and 2", snap.Files, snap.Included)
	}
	for _, want := range []string{
		`<file path="main.go">`, "func main() {}", `<file path="sub/util.go">`,
		"- logo.png (binary, not included)", "- big.txt (500 B, not included",
	} {
		if !strings.Contains(snap.Text, want) {
			t.Errorf("snapshot is missing %q:\n%s", want, snap.Text)
		}
	}
	for _, unwanted := range []string{"node_modules", ".git/config", "xxxxxxxx"} {
		if strings.Contains(snap.Text, unwanted) {
			t.Errorf("snapshot should not contain %q", unwanted)
		}
	}
}

func TestNoteResultAndEarlySummary(t *testing.T) {
	s := &Session{edited: map[string]bool{}}
	edit := fantasy.ToolCallContent{ToolName: "edit", Input: `{"path":"b.go","old_text":"x","new_text":"y"}`}
	write := fantasy.ToolCallContent{ToolName: "write", Input: `{"path":"a.go","content":"z"}`}
	failed := fantasy.ToolCallContent{ToolName: "edit", Input: `{"path":"c.go","old_text":"x","new_text":"y"}`}
	test := fantasy.ToolCallContent{ToolName: "bash", Input: `{"command":"go test ./..."}`}

	s.noteResult(edit, "replaced 1 occurrence(s) in b.go", false)
	s.noteResult(write, "wrote a.go", false)
	s.noteResult(failed, "old_text not found", true)
	if s.lastOK {
		t.Fatal("lastOK after an edit")
	}
	s.noteResult(test, "FAIL\n[exit code 1]", false)
	if s.lastOK {
		t.Fatal("lastOK after a failing command")
	}
	s.noteResult(test, "ok\n[exit code 0]", false)
	if !s.lastOK {
		t.Fatal("not lastOK after a passing command")
	}

	got := s.earlySummary()
	if !strings.Contains(got, "Changed: a.go, b.go") || strings.Contains(got, "c.go") || !strings.Contains(got, "go test ./...") {
		t.Fatalf("earlySummary = %q", got)
	}
}

func TestSystemPromptSections(t *testing.T) {
	plain := systemPrompt(Config{Dir: "/r"})
	fast := systemPrompt(Config{Dir: "/r", Snapshot: true, Batch: true})
	if strings.Contains(plain, "snapshot") || strings.Contains(plain, "single response") {
		t.Fatal("plain prompt mentions fast-flow instructions")
	}
	if !strings.Contains(fast, "repository snapshot") || !strings.Contains(fast, "single response") {
		t.Fatal("fast prompt is missing its instructions")
	}
}
