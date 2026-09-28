package jobs

import (
	"context"
	"testing"

	"example.com/kv/store"
)

func TestWarmThenCopy(t *testing.T) {
	src, dst := store.New(), store.New()
	if err := Warm(src, map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatal(err)
	}
	n, err := Copy(context.Background(), src, dst, []string{"a", "b", "c"})
	if err != nil || n != 2 {
		t.Fatalf("Copy = %d, %v", n, err)
	}
}
