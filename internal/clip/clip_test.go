package clip

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortTextIsKept(t *testing.T) {
	for name, f := range map[string]func(string, int) string{"Middle": Middle, "End": End, "Head": Head, "Tail": Tail} {
		if got := f("short", 10); got != "short" {
			t.Errorf("%s = %q", name, got)
		}
	}
}

func TestEachKeepsItsPart(t *testing.T) {
	s := strings.Repeat("a", 50) + strings.Repeat("z", 50)
	cases := []struct {
		name, got, prefix, suffix string
	}{
		{"Middle", Middle(s, 30), strings.Repeat("a", 10) + " … ", strings.Repeat("z", 20)},
		{"End", End(s, 30), strings.Repeat("a", 6) + " … ", strings.Repeat("z", 24)},
		{"Head", Head(s, 30), strings.Repeat("a", 30), "a…"},
		{"Tail", Tail("  "+s+"\n", 30), "…z", strings.Repeat("z", 30)},
	}
	for _, c := range cases {
		if !strings.HasPrefix(c.got, c.prefix) || !strings.HasSuffix(c.got, c.suffix) {
			t.Errorf("%s = %q", c.name, c.got)
		}
	}
}

func TestSplitCountsWhatItLeavesOut(t *testing.T) {
	s := strings.Repeat("x", 100)
	start, end, omitted := Split(s, 40, 10)
	if len(start) != 10 || len(end) != 30 || omitted != 60 {
		t.Errorf("Split = %d, %d, %d bytes", len(start), len(end), omitted)
	}
}

// A cut inside a character moves back to its start, and the bytes given up
// are left out rather than taken from the end of the text.
func TestHeadCutInsideACharacter(t *testing.T) {
	if got := Head("abcé0123456789XYZ", 4); got != "abc…" {
		t.Errorf("Head = %q, want %q", got, "abc…")
	}
}

func TestResultsAreValidUTF8(t *testing.T) {
	s := strings.Repeat("Crème Brûlée ñandú ", 40)
	for n := 5; n < 200; n++ {
		for name, f := range map[string]func(string, int) string{"Middle": Middle, "End": End, "Head": Head, "Tail": Tail} {
			if got := f(s, n); !utf8.ValidString(got) {
				t.Fatalf("%s(_, %d) returned invalid UTF-8: %q", name, n, got)
			}
		}
	}
	if got := Middle("ok \xff\xfe bytes", 100); !utf8.ValidString(got) {
		t.Fatalf("invalid input bytes not replaced: %q", got)
	}
}

func TestLines(t *testing.T) {
	if got := Lines("a\nb", 2); got != "a\nb" {
		t.Errorf("Lines kept %q", got)
	}
	if got := Lines("a\nb\nc\nd", 2); got != "a\nb\n… 2 more lines" {
		t.Errorf("Lines = %q", got)
	}
}
