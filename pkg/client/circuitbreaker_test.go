package client

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond, 2)

	// Test closed state - should allow requests
	if cb.State() != StateClosed {
		t.Error("Circuit should start closed")
	}

	// Test successful requests
	for i := 0; i < 5; i++ {
		err := cb.Call(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Closed circuit should allow requests: %v", err)
		}
	}

	// Test failures - should open after threshold
	for i := 0; i < 3; i++ {
		cb.Call(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != StateOpen {
		t.Error("Circuit should be open after threshold failures")
	}

	// Test open state - should reject requests
	err := cb.Call(func() error {
		return nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("Open circuit should reject requests, got: %v", err)
	}

	// Wait for timeout and test half-open
	time.Sleep(150 * time.Millisecond)

	if cb.State() != StateOpen {
		// First request should transition to half-open
		cb.Call(func() error { return nil })
	}

	// Test recovery with successful requests
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return nil
		})
	}

	if cb.State() != StateClosed {
		t.Error("Circuit should close after successful half-open requests")
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	cb := NewCircuitBreaker(5, 1*time.Second, 3)

	stats := cb.Stats()
	if stats["state"] != "closed" {
		t.Error("Initial state should be closed")
	}

	// Trigger failures
	for i := 0; i < 5; i++ {
		cb.Call(func() error {
			return errors.New("failure")
		})
	}

	stats = cb.Stats()
	if stats["state"] != "open" {
		t.Error("State should be open after failures")
	}
}

func BenchmarkCircuitBreaker(b *testing.B) {
	cb := NewCircuitBreaker(100, 1*time.Second, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Call(func() error {
			return nil
		})
	}
}
