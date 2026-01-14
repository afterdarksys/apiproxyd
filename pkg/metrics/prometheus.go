package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// PrometheusMetrics handles Prometheus metrics export
type PrometheusMetrics struct {
	mu                sync.RWMutex
	requestsTotal     int64
	requestsDuration  float64
	cacheHits         int64
	cacheMisses       int64
	bytesTransferred  int64
	errorCount        int64
	requestsByMethod  map[string]int64
	requestsByStatus  map[int]int64
	enabled           bool
}

// NewPrometheusMetrics creates a new metrics collector
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		requestsByMethod: make(map[string]int64),
		requestsByStatus: make(map[int]int64),
		enabled:          true,
	}
}

// RecordRequest records a request metric
func (p *PrometheusMetrics) RecordRequest(method string, statusCode int, duration time.Duration, cached bool, bytes int64) {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.requestsTotal++
	p.requestsDuration += duration.Seconds()
	p.bytesTransferred += bytes

	if cached {
		p.cacheHits++
	} else {
		p.cacheMisses++
	}

	if statusCode >= 400 {
		p.errorCount++
	}

	p.requestsByMethod[method]++
	p.requestsByStatus[statusCode]++
}

// ServeHTTP exports metrics in Prometheus format
func (p *PrometheusMetrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP apiproxyd_requests_total Total number of requests\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_requests_total counter\n")
	fmt.Fprintf(w, "apiproxyd_requests_total %d\n\n", p.requestsTotal)

	fmt.Fprintf(w, "# HELP apiproxyd_requests_duration_seconds Total duration of all requests\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_requests_duration_seconds counter\n")
	fmt.Fprintf(w, "apiproxyd_requests_duration_seconds %.2f\n\n", p.requestsDuration)

	fmt.Fprintf(w, "# HELP apiproxyd_cache_hits_total Total number of cache hits\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_cache_hits_total counter\n")
	fmt.Fprintf(w, "apiproxyd_cache_hits_total %d\n\n", p.cacheHits)

	fmt.Fprintf(w, "# HELP apiproxyd_cache_misses_total Total number of cache misses\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_cache_misses_total counter\n")
	fmt.Fprintf(w, "apiproxyd_cache_misses_total %d\n\n", p.cacheMisses)

	fmt.Fprintf(w, "# HELP apiproxyd_bytes_transferred_total Total bytes transferred\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_bytes_transferred_total counter\n")
	fmt.Fprintf(w, "apiproxyd_bytes_transferred_total %d\n\n", p.bytesTransferred)

	fmt.Fprintf(w, "# HELP apiproxyd_errors_total Total number of errors (4xx/5xx)\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_errors_total counter\n")
	fmt.Fprintf(w, "apiproxyd_errors_total %d\n\n", p.errorCount)

	fmt.Fprintf(w, "# HELP apiproxyd_requests_by_method Requests by HTTP method\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_requests_by_method counter\n")
	for method, count := range p.requestsByMethod {
		fmt.Fprintf(w, "apiproxyd_requests_by_method{method=\"%s\"} %d\n", method, count)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "# HELP apiproxyd_requests_by_status Requests by status code\n")
	fmt.Fprintf(w, "# TYPE apiproxyd_requests_by_status counter\n")
	for status, count := range p.requestsByStatus {
		fmt.Fprintf(w, "apiproxyd_requests_by_status{status=\"%d\"} %d\n", status, count)
	}
}
