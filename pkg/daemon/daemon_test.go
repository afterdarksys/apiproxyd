package daemon

import (
	"net/http"
	"testing"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/config"
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
