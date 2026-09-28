package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

type hiddenTodo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

func TestHiddenListJSON(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"list", "--json"}, &b); err != nil {
		t.Fatal(err)
	}
	var got []hiddenTodo
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("not a JSON array of todos: %v\n%s", err, b.String())
	}
	if len(got) != 3 || got[0].ID != 1 || got[0].Title != "Buy milk" || got[1].Done != true {
		t.Fatalf("unexpected todos: %+v", got)
	}
	var raw []map[string]any
	_ = json.Unmarshal(b.Bytes(), &raw)
	for _, k := range []string{"id", "title", "done"} {
		if _, ok := raw[0][k]; !ok {
			t.Fatalf("field %q missing: %s", k, b.String())
		}
	}
}

func TestHiddenListJSONDone(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"list", "--json", "--done"}, &b); err != nil {
		t.Fatal(err)
	}
	var got []hiddenTodo
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("want only todo 2, got %+v", got)
	}
}

func TestHiddenPlainUnchanged(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"list", "--done"}, &b); err != nil {
		t.Fatal(err)
	}
	if b.String() != "[x] 2 Write report\n" {
		t.Fatalf("plain output changed: %q", b.String())
	}
}
