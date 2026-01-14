package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements token bucket rate limiting for per-IP and per-key limits
type RateLimiter struct {
	mu            sync.RWMutex
	ipLimiters    map[string]*tokenBucket
	keyLimiters   map[string]*tokenBucket
	ipRate        int           // requests per minute
	keyRate       int           // requests per minute
	burst         int           // burst size
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// tokenBucket implements the token bucket algorithm
type tokenBucket struct {
	tokens       float64
	capacity     float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(ipRate, keyRate, burst int) *RateLimiter {
	rl := &RateLimiter{
		ipLimiters:  make(map[string]*tokenBucket),
		keyLimiters: make(map[string]*tokenBucket),
		ipRate:      ipRate,
		keyRate:     keyRate,
		burst:       burst,
		done:        make(chan struct{}),
	}

	// Start cleanup goroutine to remove stale limiters
	rl.cleanupTicker = time.NewTicker(5 * time.Minute)
	go rl.cleanup()

	return rl
}

// Middleware returns an HTTP middleware that enforces rate limits
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get client IP
		ip := getClientIP(r)

		// Check IP-based rate limit
		if !rl.allowIP(ip) {
			http.Error(w, "Rate limit exceeded for IP", http.StatusTooManyRequests)
			return
		}

		// Check API key-based rate limit if present
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" && !rl.allowKey(apiKey) {
			http.Error(w, "Rate limit exceeded for API key", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allowIP checks if a request from the given IP should be allowed
func (rl *RateLimiter) allowIP(ip string) bool {
	rl.mu.Lock()
	bucket, exists := rl.ipLimiters[ip]
	if !exists {
		bucket = newTokenBucket(rl.ipRate, rl.burst)
		rl.ipLimiters[ip] = bucket
	}
	rl.mu.Unlock()

	return bucket.allow()
}

// allowKey checks if a request with the given API key should be allowed
func (rl *RateLimiter) allowKey(key string) bool {
	rl.mu.Lock()
	bucket, exists := rl.keyLimiters[key]
	if !exists {
		bucket = newTokenBucket(rl.keyRate, rl.burst)
		rl.keyLimiters[key] = bucket
	}
	rl.mu.Unlock()

	return bucket.allow()
}

// cleanup removes stale rate limiters
func (rl *RateLimiter) cleanup() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.mu.Lock()
			now := time.Now()

			// Remove IP limiters inactive for > 10 minutes
			for ip, bucket := range rl.ipLimiters {
				bucket.mu.Lock()
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(rl.ipLimiters, ip)
				}
				bucket.mu.Unlock()
			}

			// Remove key limiters inactive for > 10 minutes
			for key, bucket := range rl.keyLimiters {
				bucket.mu.Lock()
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(rl.keyLimiters, key)
				}
				bucket.mu.Unlock()
			}

			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (rl *RateLimiter) Close() {
	rl.cleanupTicker.Stop()
	close(rl.done)
}

// Stats returns rate limiter statistics
func (rl *RateLimiter) Stats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return map[string]interface{}{
		"ip_limiters":  len(rl.ipLimiters),
		"key_limiters": len(rl.keyLimiters),
		"ip_rate":      rl.ipRate,
		"key_rate":     rl.keyRate,
		"burst":        rl.burst,
	}
}

// newTokenBucket creates a new token bucket
func newTokenBucket(ratePerMinute, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(burst),
		capacity:   float64(burst),
		refillRate: float64(ratePerMinute) / 60.0, // convert to per-second
		lastRefill: time.Now(),
	}
}

// allow checks if a request should be allowed and consumes a token
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now

	// Check if we have tokens available
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// getClientIP extracts the real client IP from the request
// Handles X-Forwarded-For and X-Real-IP headers
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For first (handles proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
