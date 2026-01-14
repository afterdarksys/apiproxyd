package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WarmingConfig defines cache warming configuration
type WarmingConfig struct {
	Enabled      bool          `json:"enabled"`
	ConfigPath   string        `json:"config_path"`
	OnStartup    bool          `json:"on_startup"`
	Schedule     string        `json:"schedule"`      // cron-like schedule
	Concurrency  int           `json:"concurrency"`   // parallel requests
	Timeout      time.Duration `json:"timeout"`       // per-request timeout
	RetryCount   int           `json:"retry_count"`
	RetryDelay   time.Duration `json:"retry_delay"`
}

// WarmingEntry defines a single endpoint to warm
type WarmingEntry struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Body     string            `json:"body,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Priority int               `json:"priority"` // higher = earlier
}

// WarmingSpec defines the cache warming specification
type WarmingSpec struct {
	Version  string          `json:"version"`
	Updated  time.Time       `json:"updated"`
	Endpoints []WarmingEntry `json:"endpoints"`
}

// Warmer handles cache warming operations
type Warmer struct {
	cache       Cache
	config      *WarmingConfig
	spec        *WarmingSpec
	client      WarmingClient
	mu          sync.RWMutex
	stats       *WarmingStats
}

// WarmingClient interface for making HTTP requests
type WarmingClient interface {
	Request(method, path string, body []byte, headers map[string]string) ([]byte, error)
}

// WarmingStats tracks cache warming statistics
type WarmingStats struct {
	LastRun       time.Time
	TotalWarmed   int64
	SuccessCount  int64
	FailureCount  int64
	Duration      time.Duration
	InProgress    bool
}

// NewWarmer creates a new cache warmer
func NewWarmer(cache Cache, config *WarmingConfig, client WarmingClient) (*Warmer, error) {
	w := &Warmer{
		cache:  cache,
		config: config,
		client: client,
		stats:  &WarmingStats{},
	}

	// Load warming spec if configured
	if config.ConfigPath != "" {
		if err := w.LoadSpec(config.ConfigPath); err != nil {
			return nil, fmt.Errorf("failed to load warming spec: %w", err)
		}
	}

	return w, nil
}

// LoadSpec loads the warming specification from a file
func (w *Warmer) LoadSpec(path string) error {
	// Expand home directory
	if path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, path[2:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read warming spec: %w", err)
	}

	var spec WarmingSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("failed to parse warming spec: %w", err)
	}

	w.mu.Lock()
	w.spec = &spec
	w.mu.Unlock()

	return nil
}

// Warm executes cache warming for all configured endpoints
func (w *Warmer) Warm(ctx context.Context) error {
	if !w.config.Enabled {
		return fmt.Errorf("cache warming is not enabled")
	}

	w.mu.Lock()
	if w.stats.InProgress {
		w.mu.Unlock()
		return fmt.Errorf("cache warming already in progress")
	}
	w.stats.InProgress = true
	w.stats.LastRun = time.Now()
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.stats.InProgress = false
		w.stats.Duration = time.Since(w.stats.LastRun)
		w.mu.Unlock()
	}()

	spec := w.getSpec()
	if spec == nil {
		return fmt.Errorf("no warming specification loaded")
	}

	// Sort by priority (highest first)
	entries := make([]WarmingEntry, len(spec.Endpoints))
	copy(entries, spec.Endpoints)
	sortByPriority(entries)

	// Create worker pool
	concurrency := w.config.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	errChan := make(chan error, len(entries))

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg.Add(1)
		go func(e WarmingEntry) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := w.warmEndpoint(ctx, e); err != nil {
				errChan <- err
				w.incrementFailures()
			} else {
				w.incrementSuccesses()
			}
		}(entry)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("cache warming completed with %d errors", len(errors))
	}

	return nil
}

// warmEndpoint warms a single endpoint
func (w *Warmer) warmEndpoint(ctx context.Context, entry WarmingEntry) error {
	timeout := w.config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Retry logic
	retries := w.config.RetryCount
	if retries <= 0 {
		retries = 2
	}
	retryDelay := w.config.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 1 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Make request
		resp, err := w.client.Request(entry.Method, entry.Path, []byte(entry.Body), entry.Headers)
		if err != nil {
			lastErr = err
			continue
		}

		// Cache the response
		cacheKey := GenerateKey(entry.Method, entry.Path, entry.Body)
		if err := w.cache.Set(cacheKey, resp); err != nil {
			lastErr = fmt.Errorf("failed to cache response: %w", err)
			continue
		}

		w.incrementTotalWarmed()
		return nil
	}

	return fmt.Errorf("failed to warm %s %s after %d attempts: %w", entry.Method, entry.Path, retries+1, lastErr)
}

// WarmEndpoints warms specific endpoints (on-demand)
func (w *Warmer) WarmEndpoints(ctx context.Context, entries []WarmingEntry) error {
	if !w.config.Enabled {
		return fmt.Errorf("cache warming is not enabled")
	}

	// Create temporary spec
	tempSpec := &WarmingSpec{
		Version:   "on-demand",
		Updated:   time.Now(),
		Endpoints: entries,
	}

	oldSpec := w.getSpec()
	w.mu.Lock()
	w.spec = tempSpec
	w.mu.Unlock()

	// Restore original spec after warming
	defer func() {
		w.mu.Lock()
		w.spec = oldSpec
		w.mu.Unlock()
	}()

	return w.Warm(ctx)
}

// Stats returns cache warming statistics
func (w *Warmer) Stats() WarmingStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return *w.stats
}

// getSpec safely retrieves the warming spec
func (w *Warmer) getSpec() *WarmingSpec {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.spec
}

// incrementSuccesses increments the success counter
func (w *Warmer) incrementSuccesses() {
	w.mu.Lock()
	w.stats.SuccessCount++
	w.mu.Unlock()
}

// incrementFailures increments the failure counter
func (w *Warmer) incrementFailures() {
	w.mu.Lock()
	w.stats.FailureCount++
	w.mu.Unlock()
}

// incrementTotalWarmed increments the total warmed counter
func (w *Warmer) incrementTotalWarmed() {
	w.mu.Lock()
	w.stats.TotalWarmed++
	w.mu.Unlock()
}

// sortByPriority sorts entries by priority (descending)
func sortByPriority(entries []WarmingEntry) {
	// Simple bubble sort (good enough for small lists)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Priority > entries[i].Priority {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// Example warming configuration file:
// {
//   "version": "1.0",
//   "updated": "2026-01-14T00:00:00Z",
//   "endpoints": [
//     {
//       "method": "GET",
//       "path": "/v1/darkapi/ip/8.8.8.8",
//       "priority": 100
//     },
//     {
//       "method": "GET",
//       "path": "/v1/darkapi/ip/1.1.1.1",
//       "priority": 90
//     }
//   ]
// }
