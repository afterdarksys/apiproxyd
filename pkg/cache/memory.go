package cache

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// MemoryCache implements an LRU in-memory cache with size limits
// This provides a fast L1 cache layer in front of the database cache (L2)
type MemoryCache struct {
	mu         sync.RWMutex
	capacity   int
	ttl        time.Duration
	staleTTL   time.Duration
	items      map[string]*list.Element
	lru        *list.List
	hits       int64
	misses     int64
	evictions  int64
	totalBytes int64
}

type memoryEntry struct {
	key       string
	value     []byte
	expiresAt time.Time
	size      int64
}

// NewMemoryCache creates a new LRU memory cache with specified capacity
func NewMemoryCache(capacity int, ttl time.Duration, staleTTL time.Duration) *MemoryCache {
	if capacity <= 0 {
		capacity = 1000 // default to 1000 entries
	}
	return &MemoryCache{
		capacity: capacity,
		ttl:      ttl,
		staleTTL: staleTTL,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a value from the memory cache
func (m *MemoryCache) Get(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	elem, exists := m.items[key]
	if !exists {
		m.misses++
		return nil, fmt.Errorf("cache miss")
	}

	entry := elem.Value.(*memoryEntry)

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		if time.Now().After(entry.expiresAt.Add(m.staleTTL)) {
			m.removeElement(elem)
		}
		m.misses++
		return nil, fmt.Errorf("cache expired")
	}

	// Move to front (most recently used)
	m.lru.MoveToFront(elem)
	m.hits++

	return entry.value, nil
}

// GetStale retrieves a potentially expired value from the memory cache
func (m *MemoryCache) GetStale(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	elem, exists := m.items[key]
	if !exists {
		return nil, fmt.Errorf("cache miss")
	}

	entry := elem.Value.(*memoryEntry)
	if time.Now().After(entry.expiresAt.Add(m.staleTTL)) {
		m.removeElement(elem)
		return nil, fmt.Errorf("stale cache expired")
	}
	return entry.value, nil
}

// Set stores a value in the memory cache using the default stale TTL for expiration
func (m *MemoryCache) Set(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key already exists
	if elem, exists := m.items[key]; exists {
		// Update existing entry
		entry := elem.Value.(*memoryEntry)
		oldSize := entry.size
		entry.value = value
		entry.expiresAt = time.Now().Add(m.ttl)
		entry.size = int64(len(value))
		m.totalBytes = m.totalBytes - oldSize + entry.size
		m.lru.MoveToFront(elem)
		return nil
	}

	// Create new entry
	entry := &memoryEntry{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(m.ttl),
		size:      int64(len(value)),
	}

	// Add to front of LRU list
	elem := m.lru.PushFront(entry)
	m.items[key] = elem
	m.totalBytes += entry.size

	// Evict if over capacity
	if m.lru.Len() > m.capacity {
		m.evictOldest()
	}

	return nil
}

// Delete removes a key from the memory cache
func (m *MemoryCache) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if elem, exists := m.items[key]; exists {
		m.removeElement(elem)
	}
	return nil
}

// Clear removes all entries from the cache
func (m *MemoryCache) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[string]*list.Element)
	m.lru.Init()
	m.totalBytes = 0
	return nil
}

// Close implements the cache.Cache interface by clearing the cache
func (m *MemoryCache) Close() error {
	return m.Clear()
}

// Stats returns cache statistics
func (m *MemoryCache) Stats() (*Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.hits + m.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(m.hits) / float64(total)
	}

	return &Stats{
		Entries:   int64(m.lru.Len()),
		SizeBytes: m.totalBytes,
		HitRate:   hitRate,
		Hits:      m.hits,
		Misses:    m.misses,
	}, nil
}

// CleanupExpired removes all expired entries
func (m *MemoryCache) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	// Iterate from back (least recently used)
	for elem := m.lru.Back(); elem != nil; {
		entry := elem.Value.(*memoryEntry)
		prev := elem.Prev()

		// Only remove if it's past the stale TTL
		if now.After(entry.expiresAt.Add(m.staleTTL)) {
			m.removeElement(elem)
			removed++
		}

		elem = prev
	}

	return removed
}

// evictOldest removes the least recently used item
func (m *MemoryCache) evictOldest() {
	elem := m.lru.Back()
	if elem != nil {
		m.removeElement(elem)
		m.evictions++
	}
}

// removeElement removes an element from the cache
func (m *MemoryCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*memoryEntry)
	delete(m.items, entry.key)
	m.lru.Remove(elem)
	m.totalBytes -= entry.size
}
