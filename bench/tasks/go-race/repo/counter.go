// Package counter counts hits per key.
package counter

// Counter counts hits per key.
type Counter struct {
	hits map[string]int
}

// New returns an empty counter.
func New() *Counter {
	return &Counter{hits: map[string]int{}}
}

// Inc adds one hit for key.
func (c *Counter) Inc(key string) {
	c.hits[key]++
}

// Get returns the hits for key.
func (c *Counter) Get(key string) int {
	return c.hits[key]
}

// Snapshot returns the current counts.
func (c *Counter) Snapshot() map[string]int {
	return c.hits
}
