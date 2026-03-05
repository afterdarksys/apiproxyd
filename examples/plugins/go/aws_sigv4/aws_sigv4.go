package main

import (
	"context"
	"fmt"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// AWSSigV4Plugin strips AWS SigV4 signatures so identical requests from different
// AWS credentials (or different times) map to the same cached payload.
type AWSSigV4Plugin struct {
	config map[string]interface{}
}

func NewPlugin() plugin.Plugin {
	return &AWSSigV4Plugin{}
}

func (p *AWSSigV4Plugin) Name() string    { return "aws_sigv4" }
func (p *AWSSigV4Plugin) Version() string { return "1.0.0" }

func (p *AWSSigV4Plugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[AWSSigV4] Initialized with config: %v\n", config)
	return nil
}

func (p *AWSSigV4Plugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	if req.Headers == nil {
		return req, true, nil
	}

	// Strip authorization strings for cache key generation.
	// This ensures that signed requests from different IAM roles or timestamps
	// pointing to the same resource will collide in the cache.
	if auth, exists := req.Headers["Authorization"]; exists {
		// Store the original authorization in metadata if we need to reconstruct it
		if req.Metadata == nil {
			req.Metadata = make(map[string]string)
		}
		req.Metadata["Original-Authorization"] = auth
		delete(req.Headers, "Authorization")
	}

	delete(req.Headers, "X-Amz-Date")
	delete(req.Headers, "X-Amz-Security-Token")

	return req, true, nil
}

func (p *AWSSigV4Plugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// A real implementation would use aws-sdk-go-v2 credentials to sign the outgoing
	// request if it was a cache miss. Since we stripped the headers in OnRequest,
	// the upstream would reject it without re-signing.
	//
	// To fully implement transparent proxy re-signing, the proxy's `doRequest` pipeline
	// would need an `OnBeforeUpstreamRequest` hook to re-inject standard auth.
	// For now, this plugin demonstrates the cache stripping.

	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Headers["X-AWS-Proxy"] = "SigV4-Stripped"

	return resp, nil
}

func (p *AWSSigV4Plugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	return resp, nil
}

func (p *AWSSigV4Plugin) Shutdown() error { return nil }

func main() {}
