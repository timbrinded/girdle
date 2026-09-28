// Package clip shortens text to a byte budget, for prompts, Jev states, tool
// output and the event log. Every result is valid UTF-8, cut only on
// character boundaries: Jev and the event log reject anything else, and tool
// output can be arbitrary bytes.
package clip

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Split shortens s to about n bytes: its first head bytes and the rest from
// its end. omitted is how many bytes were left out between the two; when it
// is 0, s fit, start is the whole of it and end is empty.
func Split(s string, n, head int) (start, end string, omitted int) {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= n {
		return s, "", 0
	}
	// The end's share is fixed before the head moves back to a character
	// boundary: bytes the head gives up are left out, not handed to the end,
	// so Head never picks up the last bytes of s.
	tail := len(s) - (n - head)
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tail < len(s) && !utf8.RuneStart(s[tail]) {
		tail++
	}
	return s[:head], s[tail:], tail - head
}

// Middle keeps the start and end of s, where the useful parts usually are.
func Middle(s string, n int) string { return cut(s, n, n/3, " … ") }

// End is Middle weighted towards the end of s, where test and build results
// appear.
func End(s string, n int) string { return cut(s, n, n/5, " … ") }

// Head keeps the start of s.
func Head(s string, n int) string { return cut(s, n, n, "…") }

// Tail keeps the end of s, without surrounding whitespace.
func Tail(s string, n int) string { return cut(strings.TrimSpace(s), n, 0, "…") }

// Lines keeps the first n lines of s and says how many more there were.
func Lines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n)
}

func cut(s string, n, head int, mark string) string {
	start, end, omitted := Split(s, n, head)
	if omitted == 0 {
		return start
	}
	return start + mark + end
}
