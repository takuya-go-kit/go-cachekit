package cachekit

import (
	"sync"
)

// DefaultBoundedCacheSize is the default maximum number of entries when NewBoundedCache is called with maxSize <= 0.
const DefaultBoundedCacheSize = 100

type boundedEntry[V any] struct {
	value V
	slot  int
}

// BoundedCache is an in-memory FIFO cache with a fixed maximum size. Eviction follows insertion order.
// Deleted keys leave a ghost slot in the ring; ghosts are skipped on eviction so FIFO order is preserved.
// Delete reclaims head ghost slots so the next Set avoids O(maxSize) eviction steps. Safe for concurrent use.
type BoundedCache[K comparable, V any] struct {
	mu       sync.RWMutex
	keys     []K
	index    map[K]boundedEntry[V]
	head     int
	ringUsed int
	maxSize  int
}

// NewBoundedCache returns a BoundedCache that holds at most maxSize entries. If maxSize <= 0, DefaultBoundedCacheSize (100) is used.
func NewBoundedCache[K comparable, V any](maxSize int) *BoundedCache[K, V] {
	if maxSize <= 0 {
		maxSize = DefaultBoundedCacheSize
	}
	return &BoundedCache[K, V]{
		keys:    make([]K, maxSize),
		index:   make(map[K]boundedEntry[V], maxSize),
		maxSize: maxSize,
	}
}

// Get returns the value for key and true if the key exists; otherwise the zero value of V and false. Nil receiver is safe and returns (zero, false).
func (c *BoundedCache[K, V]) Get(key K) (V, bool) {
	if c == nil {
		var zero V
		return zero, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.index[key]
	return e.value, ok
}

func (c *BoundedCache[K, V]) skipHeadGhosts() {
	for c.ringUsed > 0 {
		headKey := c.keys[c.head]
		if e, ok := c.index[headKey]; ok && e.slot == c.head {
			break
		}
		c.head = (c.head + 1) % c.maxSize
		c.ringUsed--
	}
}

func (c *BoundedCache[K, V]) insert(key K, value V) {
	c.skipHeadGhosts()
	for c.ringUsed >= c.maxSize {
		evictedKey := c.keys[c.head]
		if e, ok := c.index[evictedKey]; ok && e.slot == c.head {
			delete(c.index, evictedKey)
		}
		c.head = (c.head + 1) % c.maxSize
		c.ringUsed--
	}
	slot := (c.head + c.ringUsed) % c.maxSize
	c.keys[slot] = key
	c.index[key] = boundedEntry[V]{value: value, slot: slot}
	c.ringUsed++
}

// Set inserts or updates the entry for key. If the key already exists, its value is updated in place and eviction order is unchanged (FIFO by first insertion). No-op if the receiver is nil.
func (c *BoundedCache[K, V]) Set(key K, value V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.index[key]; ok {
		c.index[key] = boundedEntry[V]{value: value, slot: e.slot}
		return
	}
	c.insert(key, value)
}

// SetIfAbsent adds the entry for key only if it does not exist; does nothing if the key is already present. No-op if the receiver is nil.
func (c *BoundedCache[K, V]) SetIfAbsent(key K, value V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.index[key]; ok {
		return
	}
	c.insert(key, value)
}

// Delete removes the entry for key if present. No-op if the key is not in the cache or the receiver is nil. Head ghost slots are reclaimed so the next Set avoids O(maxSize) eviction steps.
func (c *BoundedCache[K, V]) Delete(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.index[key]; !ok {
		return
	}
	delete(c.index, key)
	c.skipHeadGhosts()
}

// Len returns the number of live entries in the cache (excludes ghost slots). Returns 0 for a nil receiver.
func (c *BoundedCache[K, V]) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.index)
}

// Cap returns the maximum number of entries the cache can hold (maxSize). Returns 0 for a nil receiver.
func (c *BoundedCache[K, V]) Cap() int {
	if c == nil {
		return 0
	}
	return c.maxSize
}
