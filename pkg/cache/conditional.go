package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

// ConditionalCache wraps a cache with conditional request support (ETags, Last-Modified)
type ConditionalCache struct {
	cache Cache
}

// CachedResponse includes metadata for conditional requests
type CachedResponse struct {
	Body         []byte
	ETag         string
	LastModified time.Time
	Headers      map[string]string
	StatusCode   int
}

// NewConditionalCache creates a cache wrapper with conditional request support
func NewConditionalCache(cache Cache) *ConditionalCache {
	return &ConditionalCache{
		cache: cache,
	}
}

// Get retrieves a cached response with conditional request headers
func (cc *ConditionalCache) Get(key string) (*CachedResponse, error) {
	// Try to get from underlying cache
	data, err := cc.cache.Get(key)
	if err != nil {
		return nil, err
	}

	// Calculate ETag from content
	etag := generateETag(data)

	// For now, use cache retrieval time as last modified
	// In production, this should be stored with the cached entry
	lastModified := time.Now()

	return &CachedResponse{
		Body:         data,
		ETag:         etag,
		LastModified: lastModified,
		Headers:      make(map[string]string),
		StatusCode:   http.StatusOK,
	}, nil
}

// Set stores a response with conditional request metadata
func (cc *ConditionalCache) Set(key string, resp *CachedResponse) error {
	// For now, just store the body
	// In production, serialize the entire CachedResponse
	return cc.cache.Set(key, resp.Body)
}

// CheckConditional checks if a request can be served with 304 Not Modified
func (cc *ConditionalCache) CheckConditional(r *http.Request, cached *CachedResponse) bool {
	// Check If-None-Match (ETag)
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		// If ETags match, content hasn't changed
		if inm == cached.ETag || inm == "*" {
			return true
		}
	}

	// Check If-Modified-Since
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		ifModifiedSince, err := http.ParseTime(ims)
		if err == nil {
			// If content hasn't been modified since the specified time
			if !cached.LastModified.After(ifModifiedSince) {
				return true
			}
		}
	}

	return false
}

// WriteConditionalResponse writes a response with conditional request headers
func (cc *ConditionalCache) WriteConditionalResponse(w http.ResponseWriter, r *http.Request, cached *CachedResponse) {
	// Set ETag and Last-Modified headers
	w.Header().Set("ETag", cached.ETag)
	w.Header().Set("Last-Modified", cached.LastModified.Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "private, must-revalidate")

	// Check if we can send 304 Not Modified
	if cc.CheckConditional(r, cached) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Send full response
	w.Header().Set("Content-Type", "application/json")
	for k, v := range cached.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(cached.StatusCode)
	w.Write(cached.Body)
}

// generateETag generates an ETag from response body
func generateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`W/"%s"`, hex.EncodeToString(hash[:])[:16])
}

// StaleEntry represents a cache entry with stale-while-revalidate support
type StaleEntry struct {
	Key          string
	Value        []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
	StaleUntil   time.Time // time until which stale content can be served
	Revalidating bool      // whether background revalidation is in progress
}

// IsStale checks if an entry is stale but within the stale-while-revalidate window
func (e *StaleEntry) IsStale() bool {
	now := time.Now()
	return now.After(e.ExpiresAt) && now.Before(e.StaleUntil)
}

// IsExpired checks if an entry is completely expired
func (e *StaleEntry) IsExpired() bool {
	return time.Now().After(e.StaleUntil)
}

// ShouldRevalidate checks if background revalidation should be triggered
func (e *StaleEntry) ShouldRevalidate() bool {
	return e.IsStale() && !e.Revalidating
}

// StaleWhileRevalidateCache implements stale-while-revalidate caching strategy
type StaleWhileRevalidateCache struct {
	cache           Cache
	staleTTL        time.Duration // how long to serve stale content
	revalidateFunc  func(key string) ([]byte, error)
	revalidating    map[string]bool
	revalidateChan  chan string
	done            chan struct{}
}

// NewStaleWhileRevalidateCache creates a new SWR cache
func NewStaleWhileRevalidateCache(cache Cache, staleTTL time.Duration, revalidateFunc func(string) ([]byte, error)) *StaleWhileRevalidateCache {
	swrc := &StaleWhileRevalidateCache{
		cache:          cache,
		staleTTL:       staleTTL,
		revalidateFunc: revalidateFunc,
		revalidating:   make(map[string]bool),
		revalidateChan: make(chan string, 100),
		done:           make(chan struct{}),
	}

	// Start background revalidation worker
	go swrc.revalidationWorker()

	return swrc
}

// Get retrieves from cache and triggers background revalidation if stale
func (swrc *StaleWhileRevalidateCache) Get(key string) ([]byte, bool, error) {
	// Try to get from cache
	data, err := swrc.cache.Get(key)
	if err != nil {
		return nil, false, err
	}

	// For now, we don't have expiry metadata, so we always return fresh
	// In production, check if entry is stale and trigger revalidation
	// This is a simplified implementation

	return data, false, nil
}

// TriggerRevalidation queues a key for background revalidation
func (swrc *StaleWhileRevalidateCache) TriggerRevalidation(key string) {
	select {
	case swrc.revalidateChan <- key:
	default:
		// Channel full, skip revalidation
	}
}

// revalidationWorker processes background revalidation requests
func (swrc *StaleWhileRevalidateCache) revalidationWorker() {
	for {
		select {
		case key := <-swrc.revalidateChan:
			// Check if already revalidating
			if swrc.isRevalidating(key) {
				continue
			}

			swrc.setRevalidating(key, true)

			// Fetch fresh data
			if data, err := swrc.revalidateFunc(key); err == nil {
				// Update cache with fresh data
				swrc.cache.Set(key, data)
			}

			swrc.setRevalidating(key, false)

		case <-swrc.done:
			return
		}
	}
}

// isRevalidating checks if a key is currently being revalidated
func (swrc *StaleWhileRevalidateCache) isRevalidating(key string) bool {
	// In production, use proper locking
	return swrc.revalidating[key]
}

// setRevalidating sets the revalidating status for a key
func (swrc *StaleWhileRevalidateCache) setRevalidating(key string, status bool) {
	// In production, use proper locking
	swrc.revalidating[key] = status
}

// Close stops the background revalidation worker
func (swrc *StaleWhileRevalidateCache) Close() error {
	close(swrc.done)
	return nil
}

// CacheControlParser parses Cache-Control headers
type CacheControlDirectives struct {
	MaxAge           int
	SMaxAge          int
	StaleWhileRevalidate int
	StaleIfError     int
	MustRevalidate   bool
	NoCache          bool
	NoStore          bool
	Public           bool
	Private          bool
}

// ParseCacheControl parses a Cache-Control header value
func ParseCacheControl(header string) *CacheControlDirectives {
	directives := &CacheControlDirectives{
		MaxAge:  -1,
		SMaxAge: -1,
		StaleWhileRevalidate: -1,
		StaleIfError: -1,
	}

	// Simple parser - in production, use a proper HTTP header parser
	// This is a placeholder implementation

	return directives
}
