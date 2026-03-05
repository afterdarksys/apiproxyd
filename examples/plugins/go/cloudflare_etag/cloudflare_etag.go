package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// CloudflareETagPlugin tracks ETags of responses to send If-None-Match headers.
// It bypasses the proxy's regular TTL and uses Cloudflare's 304 Not Modified.
type CloudflareETagPlugin struct {
	config map[string]interface{}
	// A simple thread-safe map to store ETags
	etags sync.Map
}

func NewPlugin() plugin.Plugin {
	return &CloudflareETagPlugin{}
}

func (p *CloudflareETagPlugin) Name() string    { return "cloudflare_etag" }
func (p *CloudflareETagPlugin) Version() string { return "1.0.0" }

func (p *CloudflareETagPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[CloudflareETag] Initialized with config: %v\n", config)
	return nil
}

func (p *CloudflareETagPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// If we've seen this endpoint and have an ETag, attach it
	if etagVal, ok := p.etags.Load(req.Endpoint); ok {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["If-None-Match"] = etagVal.(string)
	}

	return req, true, nil
}

func (p *CloudflareETagPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// If upstream responded with 304 Not Modified, we should ideally fetch from cache.
	// However, the proxy architecture right now requires the proxy core to handle 304s to fetch the body.
	// This plugin stores the returned ETag.
	if resp.Headers != nil {
		if etag, exists := resp.Headers["Etag"]; exists {
			p.etags.Store(req.Endpoint, etag)
		} else if etag, exists := resp.Headers["ETag"]; exists {
			p.etags.Store(req.Endpoint, etag)
		}
	}

	return resp, nil
}

func (p *CloudflareETagPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *CloudflareETagPlugin) Shutdown() error { return nil }

func main() {}
