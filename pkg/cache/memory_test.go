package cache

import (
	"testing"
	"time"
)

func TestMemoryCache(t *testing.T) {
	cache := NewMemoryCache(3)

	// Test Set and Get
	err := cache.Set("key1", []byte("value1"), 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to set: %v", err)
	}

	val, err := cache.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(val))
	}

	// Test cache miss
	_, err = cache.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for cache miss")
	}

	// Test LRU eviction
	cache.Set("key2", []byte("value2"), 1*time.Hour)
	cache.Set("key3", []byte("value3"), 1*time.Hour)
	cache.Set("key4", []byte("value4"), 1*time.Hour) // Should evict key1

	_, err = cache.Get("key1")
	if err == nil {
		t.Error("Expected key1 to be evicted")
	}

	// Test stats
	stats := cache.Stats()
	if stats.Entries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.Entries)
	}

	// Test expiration
	cache.Set("expires", []byte("soon"), 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	_, err = cache.Get("expires")
	if err == nil {
		t.Error("Expected expired entry to be removed")
	}
}

func TestMemoryCacheHitRate(t *testing.T) {
	cache := NewMemoryCache(10)

	// Generate hits and misses
	for i := 0; i < 10; i++ {
		cache.Set("key", []byte("value"), 1*time.Hour)
	}

	for i := 0; i < 10; i++ {
		cache.Get("key") // hits
	}

	for i := 0; i < 5; i++ {
		cache.Get("miss") // misses
	}

	stats := cache.Stats()
	expectedHitRate := 10.0 / 15.0
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate ~%.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func BenchmarkMemoryCacheGet(b *testing.B) {
	cache := NewMemoryCache(1000)
	cache.Set("key", []byte("value"), 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkMemoryCacheSet(b *testing.B) {
	cache := NewMemoryCache(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("key", []byte("value"), 1*time.Hour)
	}
}
