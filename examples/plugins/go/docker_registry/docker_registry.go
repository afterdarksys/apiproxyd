package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// DockerRegistryPlugin caches immutable blobs forever and caches manifests shortly.
type DockerRegistryPlugin struct {
	config map[string]interface{}
}

func NewPlugin() plugin.Plugin {
	return &DockerRegistryPlugin{}
}

func (p *DockerRegistryPlugin) Name() string    { return "docker_registry" }
func (p *DockerRegistryPlugin) Version() string { return "1.0.0" }

func (p *DockerRegistryPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[DockerRegistry] Initialized with config: %v\n", config)
	return nil
}

func (p *DockerRegistryPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Only intercept GET and HEAD requests to the registry API
	if req.Method != "GET" && req.Method != "HEAD" {
		return req, true, nil
	}

	// Strip authorization token for cache key generation so different users hit the same cache
	if req.Headers != nil {
		delete(req.Headers, "Authorization")
	}

	return req, true, nil
}

func (p *DockerRegistryPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// If it's an immutable blob, we want it cached forever.
	// We can signal this via metadata to a hypothetical advanced cache or add custom cache-control.
	if strings.Contains(req.Endpoint, "/blobs/sha256:") {
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		// Force intermediate cache to hold this for a year
		resp.Headers["Cache-Control"] = "public, max-age=31536000, immutable"
	}

	// If it's a manifest, it can change (e.g., heavily used 'latest' tag).
	if strings.Contains(req.Endpoint, "/manifests/") {
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		resp.Headers["Cache-Control"] = "public, max-age=60" // Cache for 1 minute
	}

	return resp, nil
}

func (p *DockerRegistryPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *DockerRegistryPlugin) Shutdown() error { return nil }

func main() {}
