package cache

import (
	"time"
)

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
}

// NewLayeredCache creates a new layered cache
func NewLayeredCache(dbCache Cache, memoryCacheSize int, ttl time.Duration) *LayeredCache {
	return &LayeredCache{
		l1:  NewMemoryCache(memoryCacheSize),
		l2:  dbCache,
		ttl: ttl,
	}
}

// Get retrieves a value from the cache (L1 -> L2)
func (c *LayeredCache) Get(key string) ([]byte, error) {
	// Try L1 (memory cache) first
	if value, err := c.l1.Get(key); err == nil {
		return value, nil
	}

	// L1 miss - try L2 (database cache)
	value, err := c.l2.Get(key)
	if err != nil {
		c.l2Miss++
		return nil, err
	}

	// L2 hit - promote to L1 for future requests
	c.l1Miss++
	c.l1.Set(key, value, c.ttl)

	return value, nil
}

// Set stores a value in both cache layers
func (c *LayeredCache) Set(key string, value []byte) error {
	// Store in L2 (persistent) first
	if err := c.l2.Set(key, value); err != nil {
		return err
	}

	// Then store in L1 (memory) for fast access
	return c.l1.Set(key, value, c.ttl)
}

// Delete removes a key from both cache layers
func (c *LayeredCache) Delete(key string) error {
	// Remove from L1
	c.l1.Delete(key)

	// Remove from L2
	return c.l2.Delete(key)
}

// Stats returns combined statistics from both layers
func (c *LayeredCache) Stats() (*Stats, error) {
	l1Stats := c.l1.Stats()
	l2Stats, err := c.l2.Stats()
	if err != nil {
		return nil, err
	}

	// Calculate combined hit rate
	totalHits := l1Stats.Hits + c.l1Miss
	totalMisses := c.l2Miss
	total := totalHits + totalMisses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(totalHits) / float64(total)
	}

	return &Stats{
		Entries:   l2Stats.Entries,        // L2 is the source of truth for total entries
		SizeBytes: l2Stats.SizeBytes,      // L2 size (L1 size is much smaller)
		HitRate:   hitRate,                // Combined hit rate
		Hits:      totalHits,              // L1 hits + L2 hits
		Misses:    totalMisses,            // Complete misses
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
func (c *LayeredCache) GetL1Stats() *Stats {
	return c.l1.Stats()
}

// ClearL1 clears only the L1 cache (useful for testing or cache warming)
func (c *LayeredCache) ClearL1() {
	c.l1.Clear()
}
