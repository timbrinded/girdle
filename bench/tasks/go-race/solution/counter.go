// Package counter counts hits per key.
package counter

import (
	"maps"
	"sync"
)

// Counter counts hits per key. It is safe for concurrent use.
type Counter struct {
	mu   sync.Mutex
	hits map[string]int
}

// New returns an empty counter.
func New() *Counter {
	return &Counter{hits: map[string]int{}}
}

// Inc adds one hit for key.
func (c *Counter) Inc(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits[key]++
}

// Get returns the hits for key.
func (c *Counter) Get(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits[key]
}

// Snapshot returns a copy of the current counts.
func (c *Counter) Snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.hits)
}
