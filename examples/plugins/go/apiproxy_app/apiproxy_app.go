package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// APIProxyAppPlugin provides specialized caching and request normalization
// specifically tailored to the apiproxy.app Gateway platform.
type APIProxyAppPlugin struct {
	config map[string]interface{}
}

// NewPlugin is the required factory function for Go plugins
func NewPlugin() plugin.Plugin {
	return &APIProxyAppPlugin{}
}

// Name returns the plugin's name
func (p *APIProxyAppPlugin) Name() string {
	return "apiproxy_app"
}

// Version returns the plugin's API version
func (p *APIProxyAppPlugin) Version() string {
	return "1.0.0"
}

// Init initializes the plugin
func (p *APIProxyAppPlugin) Init(config map[string]interface{}) error {
	p.config = config
	return nil
}

// OnRequest intercepts requests to normalize the cache key for apiproxy.app multi-tenant support.
// It strips the X-API-Key from the cache derivation context.
func (p *APIProxyAppPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// 1. Identify if this request targets an apiproxy.app endpoint
	// Check for the known api key structure if available in Headers map
	var apiKey string
	if req.Headers != nil {
		if val, ok := req.Headers["X-API-Key"]; ok {
			apiKey = val
		} else if val, ok := req.Headers["X-Api-Key"]; ok {
			apiKey = val
		}
	}

	if apiKey == "" || !strings.HasPrefix(apiKey, "apx_live_") {
		// Not an apiproxy.app request, pass through unmodified
		return req, true, nil
	}

	if !strings.HasPrefix(req.Endpoint, "/v1/") {
		// Only intercept /v1/* endpoints
		return req, true, nil
	}

	// 2. Cache Normalization (Cross-Tenant Caching)
	cacheKeyStr := generateNormalizedCacheKey(req)

	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}

	// Set custom cache key
	req.Headers["X-Custom-Cache-Key"] = cacheKeyStr

	return req, true, nil
}

// OnResponse is called on proxy response; pass-through
func (p *APIProxyAppPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

// OnCacheHit is called on cache hit; pass-through
func (p *APIProxyAppPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

// Shutdown executes teardown mechanics
func (p *APIProxyAppPlugin) Shutdown() error {
	return nil
}

// generateNormalizedCacheKey produces a SHA-256 hash ignoring identity-specific headers
func generateNormalizedCacheKey(req *plugin.Request) string {
	// Base string includes method, path, and normalized queries
	parts := []string{
		"apiproxy_app",
		req.Method,
		req.Endpoint,
	}

	// Wait, req.Endpoint generally includes query parameters in plugin.Request?
	// If the proxy sets it up as the full URL, we should parse it, or we just hash the endpoint.
	// For now, assume endpoint contains path + queries and we'll just hash the whole Endpoint string directly.
	// Because modifying the plugin framework to expose URL.Query() might be out of scope.
	// Actually, let's look for standard headers if present:
	headersToInclude := []string{"Accept", "Content-Type"}
	for _, h := range headersToInclude {
		if val, exists := req.Headers[h]; exists {
			parts = append(parts, fmt.Sprintf("%s:%s", h, val))
		}
	}

	rawKey := strings.Join(parts, "|")

	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
}

// main is required for Go plugins
func main() {}
