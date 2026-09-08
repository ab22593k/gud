// Package cache provides a tiny generic LRU cache with per-entry TTL and
// hit/miss counters. Max entries is mandatory so memory stays bounded.
package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry[K comparable, V any] struct {
	key     K
	val     V
	expires time.Time
}

// Cache is a goroutine-safe LRU with TTL. Zero value is not usable; use New.
type Cache[K comparable, V any] struct {
	mu     sync.Mutex
	max    int
	ttl    time.Duration
	ll     *list.List
	items  map[K]*list.Element
	hits   uint64
	misses uint64
}

// New returns a cache holding at most maxEntries entries, each expiring ttl
// after Set. maxEntries <=0 defaults to 64; ttl <=0 defaults to 30s.
func New[K comparable, V any](maxEntries int, ttl time.Duration) *Cache[K, V] {
	if maxEntries <= 0 {
		maxEntries = 64
	}

	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	return &Cache[K, V]{max: maxEntries, ttl: ttl, ll: list.New(), items: make(map[K]*list.Element)}
}

// Get returns the value for key, or miss if absent or expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	var zero V

	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		c.misses++

		return zero, false
	}

	// Single-return assertion is safe: only Set inserts, always *entry[K,V].
	e := el.Value.(*entry[K, V]) //nolint:forcetypeassert // private invariant above
	if now.After(e.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		c.misses++

		return zero, false
	}

	c.ll.MoveToFront(el)

	c.hits++

	return e.val, true
}

// Set stores val under key, evicting LRU entries beyond max.
func (c *Cache[K, V]) Set(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		// Single-return assertion is safe: only Set inserts, always *entry[K,V].
		e := el.Value.(*entry[K, V]) //nolint:forcetypeassert // private invariant above

		e.val = val
		e.expires = time.Now().Add(c.ttl)

		c.ll.MoveToFront(el)

		return
	}

	el := c.ll.PushFront(&entry[K, V]{key: key, val: val, expires: time.Now().Add(c.ttl)})
	c.items[key] = el

	for c.ll.Len() > c.max {
		back := c.ll.Back()
		if back == nil {
			break
		}

		// Single-return assertion is safe: only Set inserts, always *entry[K,V].
		be := back.Value.(*entry[K, V]) //nolint:forcetypeassert // private invariant above

		delete(c.items, be.key)
		c.ll.Remove(back)
	}
}

// Hits returns cache hits.
func (c *Cache[K, V]) Hits() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.hits
}

// Misses returns cache misses.
func (c *Cache[K, V]) Misses() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.misses
}

// Len returns live entries (may include unexpired only; expired removed on Get).
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ll.Len()
}
