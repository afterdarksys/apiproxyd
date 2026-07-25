package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// call represents an in-flight request to the L2 cache
type call struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

// LayeredCache implements a two-tier cache system:
// - L1: Fast in-memory LRU cache (limited size, volatile)
// - L2: Persistent database cache (larger, durable)
// This provides optimal performance by keeping hot data in memory
// while maintaining durability and larger capacity in the database.
type LayeredCache struct {
	l1     *MemoryCache  // Fast memory cache
	l2     Cache         // Persistent database cache
	ttl    time.Duration // Default TTL
	l1Miss int64         // L1 misses that hit L2
	l2Miss int64         // Complete cache misses

	mu    sync.Mutex
	calls map[string]*call
}

// NewLayeredCache creates a new layered cache
func NewLayeredCache(dbCache Cache, memoryCacheSize int, ttl time.Duration, staleTTL time.Duration) *LayeredCache {
	return &LayeredCache{
		l1:    NewMemoryCache(memoryCacheSize, ttl, staleTTL),
		l2:    dbCache,
		ttl:   ttl,
		calls: make(map[string]*call),
	}
}

// Get retrieves a value from the cache (L1 -> L2)
func (c *LayeredCache) Get(key string) ([]byte, error) {
	// Try L1 (memory cache) first
	if value, err := c.l1.Get(key); err == nil {
		return value, nil
	}

	// L1 miss - use SingleFlight to prevent L2 thundering herd
	c.mu.Lock()
	if c.calls == nil {
		c.calls = make(map[string]*call)
	}
	if inFlight, ok := c.calls[key]; ok {
		c.mu.Unlock()
		inFlight.wg.Wait()
		return inFlight.val, inFlight.err
	}

	inFlight := &call{}
	inFlight.wg.Add(1)
	c.calls[key] = inFlight
	c.mu.Unlock() // unlock to allow other readers to wait on this flight

	// Execute L2 fetch
	value, err := c.l2.Get(key)

	inFlight.val = value
	inFlight.err = err

	c.mu.Lock()
	delete(c.calls, key)
	c.mu.Unlock()
	inFlight.wg.Done()

	if err != nil {
		atomic.AddInt64(&c.l2Miss, 1)
		return nil, err
	}

	// L2 hit - promote to L1 for future requests
	atomic.AddInt64(&c.l1Miss, 1)
	c.l1.Set(key, value)

	return value, nil
}

// GetStale retrieves a potentially expired value from the cache (L1 -> L2)
func (c *LayeredCache) GetStale(key string) ([]byte, error) {
	// Try L1
	if value, err := c.l1.GetStale(key); err == nil {
		return value, nil
	}

	// Try L2
	return c.l2.GetStale(key)
}

// Set stores a value in both cache layers
func (c *LayeredCache) Set(key string, value []byte) error {
	// Store in L2 (persistent) first
	if err := c.l2.Set(key, value); err != nil {
		return err
	}

	// Then store in L1 (memory) for fast access
	return c.l1.Set(key, value)
}

// Delete removes a key from both cache layers
func (c *LayeredCache) Delete(key string) error {
	// Remove from L1
	c.l1.Delete(key)

	// Remove from L2
	return c.l2.Delete(key)
}

// Clear removes all entries from both cache layers.
func (c *LayeredCache) Clear() error {
	_ = c.l1.Clear()
	if clearer, ok := c.l2.(interface{ Clear() error }); ok {
		return clearer.Clear()
	}
	return nil
}

// Stats returns combined statistics from both layers
func (c *LayeredCache) Stats() (*Stats, error) {
	l1Stats, _ := c.l1.Stats()
	l2Stats, err := c.l2.Stats()
	if err != nil {
		return nil, err
	}

	// Calculate combined hit rate
	l1Misses := atomic.LoadInt64(&c.l1Miss)
	l2Misses := atomic.LoadInt64(&c.l2Miss)

	totalHits := l1Stats.Hits + l1Misses
	totalMisses := l2Misses
	total := totalHits + totalMisses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(totalHits) / float64(total)
	}

	return &Stats{
		Entries:   l2Stats.Entries,   // L2 is the source of truth for total entries
		SizeBytes: l2Stats.SizeBytes, // L2 size (L1 size is much smaller)
		HitRate:   hitRate,           // Combined hit rate
		Hits:      totalHits,         // L1 hits + L2 hits
		Misses:    totalMisses,       // Complete misses
	}, nil
}

// Close closes the underlying database cache
func (c *LayeredCache) Close() error {
	return c.l2.Close()
}

// CleanupExpired cleans up both cache layers
func (c *LayeredCache) CleanupExpired() error {
	// Cleanup L1
	c.l1.CleanupExpired()

	// Cleanup L2 (if supported)
	if cleaner, ok := c.l2.(interface{ CleanupExpired() error }); ok {
		return cleaner.CleanupExpired()
	}

	return nil
}

// GetL1Stats returns L1 (memory) cache statistics
func (c *LayeredCache) GetL1Stats() (*Stats, error) {
	return c.l1.Stats()
}

// ClearL1 clears only the L1 cache (useful for testing or cache warming)
func (c *LayeredCache) ClearL1() {
	_ = c.l1.Clear()
}
