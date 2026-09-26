// Package cache is a size-bounded LRU cache whose entries expire.
package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	key, value string
	expires    time.Time
}

// Cache is a size-bounded LRU cache with per-entry expiry.
type Cache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	order    *list.List
	items    map[string]*list.Element
	onEvict  func(key, value string)
}

// New returns an empty cache.
func New(capacity int, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{capacity: capacity, ttl: ttl, now: now, order: list.New(), items: map[string]*list.Element{}}
}

func (c *Cache) expired(e *entry) bool { return !c.now().Before(e.expires) }

func (c *Cache) remove(el *list.Element, notify bool) {
	e := el.Value.(*entry)
	c.order.Remove(el)
	delete(c.items, e.key)
	if notify && c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}

// Get returns the value for key if it is present and not expired.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*entry)
	if c.expired(e) {
		c.remove(el, true)
		return "", false
	}
	c.order.MoveToFront(el)
	return e.value, true
}

// Set adds or replaces key.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := c.now().Add(c.ttl)
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		e.value, e.expires = value, exp
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&entry{key: key, value: value, expires: exp})
	for c.order.Len() > c.capacity {
		c.remove(c.order.Back(), true)
	}
}

// Delete removes key.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return false
	}
	live := !c.expired(el.Value.(*entry))
	c.remove(el, !live)
	return live
}

// Len returns the number of live entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for el := c.order.Front(); el != nil; el = el.Next() {
		if !c.expired(el.Value.(*entry)) {
			n++
		}
	}
	return n
}

// OnEvict registers a callback for evicted and expired entries.
func (c *Cache) OnEvict(fn func(key, value string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEvict = fn
}
