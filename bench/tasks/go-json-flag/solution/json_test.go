package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestListJSON(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"list", "--json"}, &b); err != nil {
		t.Fatal(err)
	}
	var got []Todo
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || len(got) != 3 {
		t.Fatalf("bad output %q: %v", b.String(), err)
	}
}
