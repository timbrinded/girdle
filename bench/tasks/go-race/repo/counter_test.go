package counter

import (
	"sync"
	"testing"
)

func TestConcurrentInc(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 100 {
				c.Inc("a")
				_ = c.Get("a")
			}
		})
	}
	wg.Wait()
	if got := c.Get("a"); got != 5000 {
		t.Fatalf("Get = %d, want 5000", got)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	c := New()
	c.Inc("a")
	s := c.Snapshot()
	s["a"] = 100
	if got := c.Get("a"); got != 1 {
		t.Fatalf("changing the snapshot changed the counter: Get = %d", got)
	}
}

func TestSnapshotWhileWriting(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			c.Inc("b")
		}
	})
	for range 100 {
		_ = c.Snapshot()
	}
	wg.Wait()
}
