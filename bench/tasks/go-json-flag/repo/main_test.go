package main

import (
	"bytes"
	"testing"
)

func TestListPlain(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"list"}, &b); err != nil {
		t.Fatal(err)
	}
	want := "[ ] 1 Buy milk\n[x] 2 Write report\n[ ] 3 Call Sam\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}
