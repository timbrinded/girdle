package cache

import (
	"sort"
	"sync"
	"testing"
	"time"
)

type zzHiddenClock struct{ t time.Time }

func (c *zzHiddenClock) now() time.Time      { return c.t }
func (c *zzHiddenClock) add(d time.Duration) { c.t = c.t.Add(d) }
func zzHiddenNewClock() *zzHiddenClock {
	return &zzHiddenClock{t: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)}
}

func TestHiddenLRU(t *testing.T) {
	c := New(2, time.Hour, zzHiddenNewClock().now)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Get("a")
	c.Set("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should be evicted as least recently used")
	}
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatal("a should survive")
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d", c.Len())
	}
}

func TestHiddenTTL(t *testing.T) {
	clk := zzHiddenNewClock()
	c := New(10, time.Minute, clk.now)
	c.Set("a", "1")
	clk.add(59 * time.Second)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a expired too early")
	}
	clk.add(2 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have expired")
	}
	if c.Len() != 0 {
		t.Fatalf("Len = %d, want 0", c.Len())
	}
}

func TestHiddenSetResetsExpiry(t *testing.T) {
	clk := zzHiddenNewClock()
	c := New(10, time.Minute, clk.now)
	c.Set("a", "1")
	clk.add(50 * time.Second)
	c.Set("a", "2")
	clk.add(50 * time.Second)
	if v, ok := c.Get("a"); !ok || v != "2" {
		t.Fatal("Set should reset expiry")
	}
}

func TestHiddenGetDoesNotResetExpiry(t *testing.T) {
	clk := zzHiddenNewClock()
	c := New(10, time.Minute, clk.now)
	c.Set("a", "1")
	clk.add(50 * time.Second)
	c.Get("a")
	clk.add(20 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Fatal("Get must not extend expiry")
	}
}

func TestHiddenOnEvict(t *testing.T) {
	clk := zzHiddenNewClock()
	c := New(1, time.Minute, clk.now)
	var got []string
	c.OnEvict(func(k, v string) { got = append(got, k+"="+v) })
	c.Set("a", "1")
	c.Set("a", "1b")
	c.Set("b", "2")
	c.Delete("b")
	c.Set("c", "3")
	clk.add(2 * time.Minute)
	c.Get("c")
	sort.Strings(got)
	want := []string{"a=1b", "c=3"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("evictions = %v, want %v", got, want)
	}
}

func TestHiddenDelete(t *testing.T) {
	clk := zzHiddenNewClock()
	c := New(10, time.Minute, clk.now)
	c.Set("a", "1")
	if !c.Delete("a") || c.Delete("a") {
		t.Fatal("Delete return values")
	}
	c.Set("b", "2")
	clk.add(2 * time.Minute)
	if c.Delete("b") {
		t.Fatal("Delete of expired key should return false")
	}
}

func TestHiddenNilClock(t *testing.T) {
	c := New(2, time.Hour, nil)
	c.Set("a", "1")
	if _, ok := c.Get("a"); !ok {
		t.Fatal("nil clock should use time.Now")
	}
}

func TestHiddenConcurrent(t *testing.T) {
	c := New(50, time.Hour, nil)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			for j := range 200 {
				k := string(rune('a' + (i+j)%26))
				c.Set(k, k)
				c.Get(k)
				c.Len()
			}
		})
	}
	wg.Wait()
}
