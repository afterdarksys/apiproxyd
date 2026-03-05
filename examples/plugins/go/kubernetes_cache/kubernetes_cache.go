package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// KubernetesCachePlugin materializes a view of specific Kubernetes API GETs by
// maintaining a background Watch. (Mock implementation)
type KubernetesCachePlugin struct {
	config map[string]interface{}
	// materialized view
	store sync.Map
}

func NewPlugin() plugin.Plugin {
	return &KubernetesCachePlugin{}
}

func (p *KubernetesCachePlugin) Name() string    { return "kubernetes_cache" }
func (p *KubernetesCachePlugin) Version() string { return "1.0.0" }

func (p *KubernetesCachePlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[KubernetesCache] Initialized with config: %v\n", config)

	// Simulate background watch populating the materialized view
	go func() {
		// In a real implementation we would stream the API
		// GET /api/v1/pods?watch=true
		for {
			time.Sleep(10 * time.Second)
			// Mock population
			p.store.Store("/api/v1/pods", []byte(`{"kind":"PodList", "items":[]}`))
		}
	}()

	return nil
}

func (p *KubernetesCachePlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Bypass cache for NON-GET verbs
	if req.Method != "GET" {
		return req, true, nil
	}

	// Serve from materialized view if available
	if strings.Contains(req.Endpoint, "/api/v1/pods") {
		if val, ok := p.store.Load(req.Endpoint); ok {
			// We can short-circuit the request returning mocked response using
			// the plugin interface by modifying the request to an endpoint
			// guaranteed to hit the proxy cache, or we can use the plugin model
			// directly. Wait, the plugin interface OnRequest only modifies the *Request.
			// Currently, the apiproxyd OnRequest doesn't let plugins return a direct response.
			// To fake a materialized view, we can just let it cache normally, or
			// if the proxy supported it, return `cont = false` and write to the ResponseWriter.
			// For this plugin, we'll mark a custom header.
			_ = val
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["X-K8s-Materialized"] = "true"
		}
	}

	return req, true, nil
}

func (p *KubernetesCachePlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// If the upstream responded matching our watch list, update our materialized view immediately
	if req.Method == "GET" && strings.Contains(req.Endpoint, "/api/v1/pods") {
		p.store.Store(req.Endpoint, resp.Body)
	}
	return resp, nil
}

func (p *KubernetesCachePlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *KubernetesCachePlugin) Shutdown() error { return nil }

func main() {}
