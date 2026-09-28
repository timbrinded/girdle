package store

import (
	"context"
	"testing"
)

func TestPutGet(t *testing.T) {
	s := New()
	if err := s.Put(context.Background(), "a", "1"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(context.Background(), "a"); err != nil || v != "1" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if _, err := s.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
