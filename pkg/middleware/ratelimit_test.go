package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(60, 300, 10) // 60/min per IP, 300/min per key
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test IP-based rate limiting
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First request should succeed
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("First request should succeed, got %d", w.Code)
	}

	// Exhaust rate limit
	for i := 0; i < 70; i++ {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Should be rate limited now
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Should be rate limited, got %d", w.Code)
	}
}

func TestTokenBucket(t *testing.T) {
	bucket := newTokenBucket(60, 10) // 60/min = 1/sec, burst 10

	// Test burst allowance
	for i := 0; i < 10; i++ {
		if !bucket.allow() {
			t.Errorf("Burst token %d should be allowed", i)
		}
	}

	// Should be rate limited
	if bucket.allow() {
		t.Error("Should be rate limited after burst")
	}

	// Wait for refill
	time.Sleep(1100 * time.Millisecond)

	// Should allow one more request
	if !bucket.allow() {
		t.Error("Should allow request after refill")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remoteAddr string
		expected string
	}{
		{
			name:     "Direct connection",
			remoteAddr: "192.168.1.1:12345",
			expected: "192.168.1.1",
		},
		{
			name:     "X-Forwarded-For",
			headers:  map[string]string{"X-Forwarded-For": "10.0.0.1, 192.168.1.1"},
			remoteAddr: "192.168.1.1:12345",
			expected: "10.0.0.1",
		},
		{
			name:     "X-Real-IP",
			headers:  map[string]string{"X-Real-IP": "10.0.0.2"},
			remoteAddr: "192.168.1.1:12345",
			expected: "10.0.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := getClientIP(req)
			if ip != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, ip)
			}
		})
	}
}

func BenchmarkRateLimiter(b *testing.B) {
	rl := NewRateLimiter(10000, 30000, 100)
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
