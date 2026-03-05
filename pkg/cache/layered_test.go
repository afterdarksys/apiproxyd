package cache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockCache is a simple mock for testing L2 cache behaviors
type mockCache struct {
	getCalled int64
	data      map[string][]byte
	delay     time.Duration
	mu        sync.RWMutex
}

func (m *mockCache) Get(key string) ([]byte, error) {
	atomic.AddInt64(&m.getCalled, 1)
	time.Sleep(m.delay) // simulate slow DB call
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.data[key]; ok {
		return val, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockCache) GetStale(key string) ([]byte, error) {
	return m.Get(key)
}

func (m *mockCache) Set(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCache) Stats() (*Stats, error) {
	return &Stats{}, nil
}

func (m *mockCache) Close() error { return nil }

func TestSingleFlightL2Cache(t *testing.T) {
	l2 := &mockCache{
		data:  map[string][]byte{"hot_key": []byte("hot_value")},
		delay: 50 * time.Millisecond,
	}

	layered := NewLayeredCache(l2, 10, time.Minute, time.Hour)

	// Simulate 100 concurrent requests for the same key
	var wg sync.WaitGroup
	var successCount int64

	numRequests := 100

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := layered.Get("hot_key")
			if err == nil && string(val) == "hot_value" {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != int64(numRequests) {
		t.Fatalf("Expected %d successful reads, got %d", numRequests, successCount)
	}

	// Because of SingleFlight, L2 should only be hit ONCE, even with 100 concurrent requests!
	if l2.getCalled != 1 {
		t.Fatalf("Thundering herd protection failed! Expected 1 L2 call, got %d", l2.getCalled)
	}
}
