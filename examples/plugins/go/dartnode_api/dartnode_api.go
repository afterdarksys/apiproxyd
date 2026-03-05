package main

import (
	"context"
	"fmt"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// DartnodeAPIPlugin handles specifics of Dartnode's backend API,
// normalizing specific custom auth tokens for better caching.
type DartnodeAPIPlugin struct {
	config map[string]interface{}
}

func NewPlugin() plugin.Plugin {
	return &DartnodeAPIPlugin{}
}

func (p *DartnodeAPIPlugin) Name() string    { return "dartnode_api" }
func (p *DartnodeAPIPlugin) Version() string { return "1.0.0" }

func (p *DartnodeAPIPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[DartnodeAPI] Initialized with config: %v\n", config)
	return nil
}

func (p *DartnodeAPIPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// If the request is using a temporary identity token, strip it
	// and replace it with a canonical one to ensure a cache hit.
	if req.Headers != nil {
		if token := req.Headers["X-Dartnode-Token"]; token != "" {
			req.Headers["X-Dartnode-Token"] = "canonicalized-token-for-cache"
		}
	}
	return req, true, nil
}

func (p *DartnodeAPIPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *DartnodeAPIPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *DartnodeAPIPlugin) Shutdown() error { return nil }

func main() {}
