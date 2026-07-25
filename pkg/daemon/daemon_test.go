package daemon

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/afterdarksys/apiproxyd/pkg/metrics"
)

func TestSemanticDeduplication(t *testing.T) {
	// Create mock config with semantic deduplication enabled
	d := &Daemon{
		cfg: &config.Config{
			Cache: config.CacheConfig{
				SemanticDeduplication: true,
			},
		},
	}

	req1, _ := http.NewRequest("GET", "/api/test?b=2&a=1&c=3", nil)
	req2, _ := http.NewRequest("GET", "/api/test?c=3&a=1&b=2", nil)

	endpoint1 := "/test"
	if d.cfg.Cache.SemanticDeduplication && len(req1.URL.Query()) > 0 {
		endpoint1 = endpoint1 + "?" + req1.URL.Query().Encode()
	}

	endpoint2 := "/test"
	if d.cfg.Cache.SemanticDeduplication && len(req2.URL.Query()) > 0 {
		endpoint2 = endpoint2 + "?" + req2.URL.Query().Encode()
	}

	if endpoint1 != endpoint2 {
		t.Fatalf("Semantic deduplication failed. Expected endpoints to match, got %s and %s", endpoint1, endpoint2)
	}

	cacheKey1 := cache.GenerateKey(req1.Method, endpoint1, "")
	cacheKey2 := cache.GenerateKey(req2.Method, endpoint2, "")

	if cacheKey1 != cacheKey2 {
		t.Fatalf("Semantic deduplication failed to produce identical cache keys")
	}
}

func TestOfflineEndpointCacheMissFetchesAndCaches(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_, _ = io.WriteString(w, `{"source":"upstream"}`)
	}))
	defer upstream.Close()

	clientCfg := client.DefaultClientConfig()
	clientCfg.CircuitBreakerEnabled = false
	clientCfg.DeduplicationEnabled = false
	upstreamClient := client.NewWithConfig("test-key", clientCfg)
	upstreamClient.BaseURL = upstream.URL

	memory := cache.NewMemoryCache(10, time.Hour, time.Hour)
	d := &Daemon{
		cache:   memory,
		client:  upstreamClient,
		metrics: metrics.NewPrometheusMetrics(),
		cfg: &config.Config{
			Cache: config.CacheConfig{
				StaleIfError: true,
			},
			Security: config.SecurityConfig{
				MaxRequestBodySize: 1024,
			},
			OfflineEndpoints:     []string{"/offline/*"},
			WhitelistedEndpoints: []string{"/offline/*"},
		},
	}

	for _, check := range []struct {
		wantCache string
		wantCalls int
	}{
		{wantCache: "MISS", wantCalls: 1},
		{wantCache: "HIT", wantCalls: 1},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/offline/item", nil)
		resp := httptest.NewRecorder()
		d.handleProxy(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
		}
		if got := resp.Header().Get("X-Cache"); got != check.wantCache {
			t.Fatalf("expected cache %s, got %s", check.wantCache, got)
		}
		if upstreamCalls != check.wantCalls {
			t.Fatalf("expected %d upstream call(s), got %d", check.wantCalls, upstreamCalls)
		}
	}
}

func TestCacheClearRequiresMutationMethodAndClearsEntries(t *testing.T) {
	memory := cache.NewMemoryCache(10, time.Hour, time.Hour)
	if err := memory.Set("key", []byte("value")); err != nil {
		t.Fatal(err)
	}
	d := &Daemon{cache: memory}

	getResp := httptest.NewRecorder()
	d.handleCacheClear(getResp, httptest.NewRequest(http.MethodGet, "/cache/clear", nil))
	if getResp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET to be rejected, got %d", getResp.Code)
	}

	postResp := httptest.NewRecorder()
	d.handleCacheClear(postResp, httptest.NewRequest(http.MethodPost, "/cache/clear", strings.NewReader("")))
	if postResp.Code != http.StatusOK {
		t.Fatalf("expected POST to clear cache, got %d", postResp.Code)
	}
	if _, err := memory.Get("key"); err == nil {
		t.Fatal("expected cached entry to be removed")
	}
}

func TestMutationRequestsAreNeverCached(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_, _ = io.WriteString(w, `{"mutated":true}`)
	}))
	defer upstream.Close()

	clientCfg := client.DefaultClientConfig()
	clientCfg.CircuitBreakerEnabled = false
	clientCfg.DeduplicationEnabled = false
	upstreamClient := client.NewWithConfig("test-key", clientCfg)
	upstreamClient.BaseURL = upstream.URL

	memory := cache.NewMemoryCache(10, time.Hour, time.Hour)
	d := &Daemon{
		cache:   memory,
		client:  upstreamClient,
		metrics: metrics.NewPrometheusMetrics(),
		cfg: &config.Config{
			Security:             config.SecurityConfig{MaxRequestBodySize: 1024},
			WhitelistedEndpoints: []string{"/mutate"},
		},
	}

	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/mutate", strings.NewReader(`{"value":1}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		d.handleProxy(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
		}
	}
	if upstreamCalls != 2 {
		t.Fatalf("expected both mutations to reach upstream, got %d calls", upstreamCalls)
	}
	stats, err := memory.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 0 {
		t.Fatalf("expected no cached mutation response, got %d entries", stats.Entries)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "[::1]"} {
		if !isLoopbackHost(host) {
			t.Fatalf("expected %q to be loopback", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "::", "192.168.1.10", "example.com"} {
		if isLoopbackHost(host) {
			t.Fatalf("expected %q not to be loopback", host)
		}
	}
}
