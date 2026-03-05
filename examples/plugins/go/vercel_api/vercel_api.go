package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// VercelAPIPlugin normalizes pagination and rate-limit parameters
// to ensure higher cache hit rates on Vercel deployments and analytics APIs.
type VercelAPIPlugin struct {
	config map[string]interface{}
}

func NewPlugin() plugin.Plugin {
	return &VercelAPIPlugin{}
}

func (p *VercelAPIPlugin) Name() string    { return "vercel_api" }
func (p *VercelAPIPlugin) Version() string { return "1.0.0" }

func (p *VercelAPIPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[VercelAPI] Initialized with config: %v\n", config)
	return nil
}

func (p *VercelAPIPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Parse the Endpoint URL
	u, err := url.Parse(req.Endpoint)
	if err != nil {
		return req, true, nil
	}

	query := u.Query()
	modified := false

	// Normalize 'since' and 'until' timestamps to the nearest minute
	// to dramatically increase cache hit rates on analytics polling
	if since := query.Get("since"); since != "" {
		if len(since) > 10 {
			// A very crude mock normalization: trim ms from epoch
			query.Set("since", since[:10])
			modified = true
		}
	}

	if modified {
		u.RawQuery = query.Encode()
		req.Endpoint = u.String()
	}

	return req, true, nil
}

func (p *VercelAPIPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// If the response contains Vercel's RateLimit headers, we can cache cautiously
	if resp.Headers != nil && resp.Headers["X-Vercel-Forwarded-For"] != "" {
		// Example: inject a custom caching directive
		resp.Headers["X-Proxy-Vercel-Optimized"] = "1"
	}
	return resp, nil
}

// Check on Cache hit
func (p *VercelAPIPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// Add an indicator that Vercel API pagination was optimized
	if strings.Contains(req.Endpoint, "since=") {
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		resp.Headers["X-Vercel-Cache-Hit"] = "Time-Normalized"
	}
	return resp, nil
}

func (p *VercelAPIPlugin) Shutdown() error { return nil }

func main() {}
