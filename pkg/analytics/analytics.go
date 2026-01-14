package analytics

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Analytics tracks cache usage and provides insights
type Analytics struct {
	mu              sync.RWMutex
	requests        int64
	cacheHits       int64
	cacheMisses     int64
	totalLatency    time.Duration
	totalBytes      int64
	savedBytes      int64 // bytes saved by serving from cache
	endpoints       map[string]*EndpointStats
	hourlyStats     map[int64]*HourlyStats
	costSavings     float64
	startTime       time.Time
}

// EndpointStats tracks statistics for a specific endpoint
type EndpointStats struct {
	Path         string
	Requests     int64
	CacheHits    int64
	CacheMisses  int64
	AvgLatency   time.Duration
	TotalLatency time.Duration
	BytesServed  int64
	LastAccess   time.Time
}

// HourlyStats tracks statistics per hour
type HourlyStats struct {
	Hour        time.Time
	Requests    int64
	CacheHits   int64
	CacheMisses int64
	BytesServed int64
}

// Summary provides a snapshot of analytics
type Summary struct {
	TotalRequests   int64         `json:"total_requests"`
	CacheHits       int64         `json:"cache_hits"`
	CacheMisses     int64         `json:"cache_misses"`
	HitRate         float64       `json:"hit_rate"`
	AvgLatency      string        `json:"avg_latency"`
	TotalBytes      int64         `json:"total_bytes"`
	SavedBytes      int64         `json:"saved_bytes"`
	CostSavings     float64       `json:"cost_savings"`
	TopEndpoints    []EndpointStats `json:"top_endpoints"`
	HourlyBreakdown []HourlyStats   `json:"hourly_breakdown"`
	Uptime          string        `json:"uptime"`
}

// NewAnalytics creates a new analytics tracker
func NewAnalytics() *Analytics {
	return &Analytics{
		endpoints:   make(map[string]*EndpointStats),
		hourlyStats: make(map[int64]*HourlyStats),
		startTime:   time.Now(),
	}
}

// RecordRequest records a request event
func (a *Analytics) RecordRequest(path string, cached bool, latency time.Duration, bytes int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.requests++
	a.totalLatency += latency
	a.totalBytes += bytes

	if cached {
		a.cacheHits++
		// Estimate bytes saved (assume upstream would be similar size)
		a.savedBytes += bytes
		// Estimate cost savings (example: $0.001 per MB)
		a.costSavings += float64(bytes) / 1024.0 / 1024.0 * 0.001
	} else {
		a.cacheMisses++
	}

	// Update endpoint stats
	if stats, exists := a.endpoints[path]; exists {
		stats.Requests++
		stats.TotalLatency += latency
		stats.AvgLatency = stats.TotalLatency / time.Duration(stats.Requests)
		stats.BytesServed += bytes
		stats.LastAccess = time.Now()
		if cached {
			stats.CacheHits++
		} else {
			stats.CacheMisses++
		}
	} else {
		a.endpoints[path] = &EndpointStats{
			Path:         path,
			Requests:     1,
			CacheHits:    0,
			CacheMisses:  0,
			TotalLatency: latency,
			AvgLatency:   latency,
			BytesServed:  bytes,
			LastAccess:   time.Now(),
		}
		if cached {
			a.endpoints[path].CacheHits = 1
		} else {
			a.endpoints[path].CacheMisses = 1
		}
	}

	// Update hourly stats
	hour := time.Now().Truncate(time.Hour).Unix()
	if stats, exists := a.hourlyStats[hour]; exists {
		stats.Requests++
		stats.BytesServed += bytes
		if cached {
			stats.CacheHits++
		} else {
			stats.CacheMisses++
		}
	} else {
		a.hourlyStats[hour] = &HourlyStats{
			Hour:        time.Unix(hour, 0),
			Requests:    1,
			CacheHits:   0,
			CacheMisses: 0,
			BytesServed: bytes,
		}
		if cached {
			a.hourlyStats[hour].CacheHits = 1
		} else {
			a.hourlyStats[hour].CacheMisses = 1
		}
	}
}

// GetSummary returns a summary of analytics
func (a *Analytics) GetSummary(topN int) *Summary {
	a.mu.RLock()
	defer a.mu.RUnlock()

	summary := &Summary{
		TotalRequests: a.requests,
		CacheHits:     a.cacheHits,
		CacheMisses:   a.cacheMisses,
		TotalBytes:    a.totalBytes,
		SavedBytes:    a.savedBytes,
		CostSavings:   a.costSavings,
		Uptime:        time.Since(a.startTime).String(),
	}

	// Calculate hit rate
	if a.requests > 0 {
		summary.HitRate = float64(a.cacheHits) / float64(a.requests)
	}

	// Calculate average latency
	if a.requests > 0 {
		avgLatency := a.totalLatency / time.Duration(a.requests)
		summary.AvgLatency = avgLatency.String()
	}

	// Get top endpoints
	endpoints := make([]EndpointStats, 0, len(a.endpoints))
	for _, stats := range a.endpoints {
		endpoints = append(endpoints, *stats)
	}

	// Sort by request count
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Requests > endpoints[j].Requests
	})

	// Take top N
	if topN > len(endpoints) {
		topN = len(endpoints)
	}
	summary.TopEndpoints = endpoints[:topN]

	// Get hourly breakdown (last 24 hours)
	hourlyBreakdown := make([]HourlyStats, 0)
	cutoff := time.Now().Add(-24 * time.Hour).Truncate(time.Hour).Unix()
	for hour, stats := range a.hourlyStats {
		if hour >= cutoff {
			hourlyBreakdown = append(hourlyBreakdown, *stats)
		}
	}

	// Sort by hour
	sort.Slice(hourlyBreakdown, func(i, j int) bool {
		return hourlyBreakdown[i].Hour.Before(hourlyBreakdown[j].Hour)
	})
	summary.HourlyBreakdown = hourlyBreakdown

	return summary
}

// GetEndpointStats returns statistics for a specific endpoint
func (a *Analytics) GetEndpointStats(path string) (*EndpointStats, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if stats, exists := a.endpoints[path]; exists {
		statsCopy := *stats
		return &statsCopy, nil
	}

	return nil, fmt.Errorf("no statistics found for endpoint: %s", path)
}

// GetTopEndpoints returns the top N endpoints by request count
func (a *Analytics) GetTopEndpoints(n int) []EndpointStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	endpoints := make([]EndpointStats, 0, len(a.endpoints))
	for _, stats := range a.endpoints {
		endpoints = append(endpoints, *stats)
	}

	// Sort by request count
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Requests > endpoints[j].Requests
	})

	if n > len(endpoints) {
		n = len(endpoints)
	}

	return endpoints[:n]
}

// GetHourlyStats returns hourly statistics for the last N hours
func (a *Analytics) GetHourlyStats(hours int) []HourlyStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Truncate(time.Hour).Unix()

	stats := make([]HourlyStats, 0)
	for hour, s := range a.hourlyStats {
		if hour >= cutoff {
			stats = append(stats, *s)
		}
	}

	// Sort by hour
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Hour.Before(stats[j].Hour)
	})

	return stats
}

// Reset resets all analytics data
func (a *Analytics) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.requests = 0
	a.cacheHits = 0
	a.cacheMisses = 0
	a.totalLatency = 0
	a.totalBytes = 0
	a.savedBytes = 0
	a.costSavings = 0
	a.endpoints = make(map[string]*EndpointStats)
	a.hourlyStats = make(map[int64]*HourlyStats)
	a.startTime = time.Now()
}

// Export exports analytics data as JSON
func (a *Analytics) Export() ([]byte, error) {
	summary := a.GetSummary(20)
	return json.MarshalIndent(summary, "", "  ")
}

// CleanupOldHourlyStats removes hourly stats older than the specified duration
func (a *Analytics) CleanupOldHourlyStats(maxAge time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Truncate(time.Hour).Unix()

	for hour := range a.hourlyStats {
		if hour < cutoff {
			delete(a.hourlyStats, hour)
		}
	}
}

// CostEstimate estimates cost savings based on cache hit rate
func (a *Analytics) CostEstimate(costPerRequest float64) map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	withoutCache := float64(a.requests) * costPerRequest
	withCache := float64(a.cacheMisses) * costPerRequest
	savings := withoutCache - withCache
	savingsPercent := 0.0
	if withoutCache > 0 {
		savingsPercent = (savings / withoutCache) * 100
	}

	return map[string]interface{}{
		"total_requests":       a.requests,
		"cache_hits":           a.cacheHits,
		"cache_misses":         a.cacheMisses,
		"cost_per_request":     costPerRequest,
		"cost_without_cache":   withoutCache,
		"cost_with_cache":      withCache,
		"cost_savings":         savings,
		"cost_savings_percent": savingsPercent,
	}
}

// PerformanceMetrics returns performance-related metrics
func (a *Analytics) PerformanceMetrics() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	avgLatency := time.Duration(0)
	if a.requests > 0 {
		avgLatency = a.totalLatency / time.Duration(a.requests)
	}

	hitRate := 0.0
	if a.requests > 0 {
		hitRate = float64(a.cacheHits) / float64(a.requests)
	}

	return map[string]interface{}{
		"total_requests":  a.requests,
		"cache_hit_rate":  hitRate,
		"avg_latency_ms":  avgLatency.Milliseconds(),
		"total_bytes":     a.totalBytes,
		"saved_bytes":     a.savedBytes,
		"uptime_seconds":  time.Since(a.startTime).Seconds(),
	}
}
