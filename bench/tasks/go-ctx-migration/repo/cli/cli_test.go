package cli

import (
	"bytes"
	"testing"

	"example.com/kv/store"
)

func TestSetGet(t *testing.T) {
	s := store.New()
	var b bytes.Buffer
	if err := Run(s, []string{"set", "k", "v"}, &b); err != nil {
		t.Fatal(err)
	}
	if err := Run(s, []string{"get", "k"}, &b); err != nil || b.String() != "v\n" {
		t.Fatalf("got %q, %v", b.String(), err)
	}
}
