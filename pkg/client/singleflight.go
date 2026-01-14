package client

import (
	"sync"
)

// SingleFlight prevents duplicate requests for the same key
// When multiple concurrent requests arrive for the same resource,
// only one actually executes while others wait for the result.
// This is critical for preventing thundering herd problems.
type SingleFlight struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

// NewSingleFlight creates a new single flight instance
func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		calls: make(map[string]*call),
	}
}

// Do executes a function, deduplicating concurrent calls with the same key
func (sf *SingleFlight) Do(key string, fn func() ([]byte, error)) ([]byte, error) {
	sf.mu.Lock()

	// Check if there's already a call in flight for this key
	if c, ok := sf.calls[key]; ok {
		sf.mu.Unlock()
		// Wait for the in-flight call to complete
		c.wg.Wait()
		return c.val, c.err
	}

	// Create new call
	c := &call{}
	c.wg.Add(1)
	sf.calls[key] = c
	sf.mu.Unlock()

	// Execute the function
	c.val, c.err = fn()

	// Mark as done and cleanup
	sf.mu.Lock()
	delete(sf.calls, key)
	sf.mu.Unlock()
	c.wg.Done()

	return c.val, c.err
}

// Stats returns statistics about in-flight requests
func (sf *SingleFlight) Stats() map[string]interface{} {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	return map[string]interface{}{
		"in_flight": len(sf.calls),
	}
}
