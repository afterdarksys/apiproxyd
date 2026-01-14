package client

import (
	"errors"
	"sync"
	"time"
)

// CircuitBreaker implements the circuit breaker pattern to prevent cascading failures
// States: Closed (normal) -> Open (failing) -> Half-Open (testing) -> Closed
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	threshold       int           // failures before opening
	timeout         time.Duration // time to wait before half-open
	halfOpenMax     int           // max requests in half-open state
}

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration, halfOpenMax int) *CircuitBreaker {
	return &CircuitBreaker{
		state:       StateClosed,
		threshold:   threshold,
		timeout:     timeout,
		halfOpenMax: halfOpenMax,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err == nil)
	return err
}

// allowRequest checks if the request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if timeout has elapsed to try half-open
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.failureCount = 0
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited requests in half-open state
		return cb.successCount+cb.failureCount < cb.halfOpenMax
	}
	return false
}

// recordResult records the result of a request
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if success {
		cb.successCount++
		cb.failureCount = 0

		// If in half-open and got enough successes, close the circuit
		if cb.state == StateHalfOpen && cb.successCount >= cb.halfOpenMax {
			cb.state = StateClosed
		}
	} else {
		cb.failureCount++
		cb.lastFailureTime = time.Now()

		// Open circuit if threshold exceeded
		if cb.failureCount >= cb.threshold {
			cb.state = StateOpen
		}
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	stateStr := "closed"
	switch cb.state {
	case StateOpen:
		stateStr = "open"
	case StateHalfOpen:
		stateStr = "half-open"
	}

	return map[string]interface{}{
		"state":         stateStr,
		"failures":      cb.failureCount,
		"successes":     cb.successCount,
		"last_failure":  cb.lastFailureTime,
	}
}
