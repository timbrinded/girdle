// Package cache is a size-bounded LRU cache whose entries expire.
//
// Requirements:
//   - New(capacity, ttl, now) returns a cache holding at most capacity
//     entries. Each entry expires ttl after it was last Set. now is the clock;
//     if nil, time.Now is used.
//   - Get(key) returns the value and true if the key is present and not
//     expired. A successful Get marks the entry as most recently used. Get
//     never returns an expired entry.
//   - Set(key, value) adds or replaces an entry, resets its expiry and marks
//     it most recently used. If the cache is then over capacity, the least
//     recently used entry is evicted.
//   - Delete(key) removes an entry. It returns true if the key was present
//     and not expired.
//   - Len() returns the number of entries that are not expired.
//   - OnEvict(fn) registers a callback, called with the key and value of
//     every entry evicted for capacity or found expired. It is not called for
//     Delete or for a Set that replaces an existing key.
//   - All methods are safe for concurrent use.
package cache

import "time"

// Cache is a size-bounded LRU cache with per-entry expiry.
type Cache struct{}

// New returns an empty cache.
func New(capacity int, ttl time.Duration, now func() time.Time) *Cache {
	panic("not implemented")
}

// Get returns the value for key if it is present and not expired.
func (c *Cache) Get(key string) (string, bool) { panic("not implemented") }

// Set adds or replaces key.
func (c *Cache) Set(key, value string) { panic("not implemented") }

// Delete removes key.
func (c *Cache) Delete(key string) bool { panic("not implemented") }

// Len returns the number of live entries.
func (c *Cache) Len() int { panic("not implemented") }

// OnEvict registers a callback for evicted and expired entries.
func (c *Cache) OnEvict(fn func(key, value string)) { panic("not implemented") }
