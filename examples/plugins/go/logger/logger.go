package main

import (
	"context"
	"fmt"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// LoggerPlugin is an example plugin that logs all requests and responses
type LoggerPlugin struct {
	config map[string]interface{}
}

// NewPlugin is the required factory function for Go plugins
func NewPlugin() plugin.Plugin {
	return &LoggerPlugin{}
}

func (l *LoggerPlugin) Name() string {
	return "logger"
}

func (l *LoggerPlugin) Version() string {
	return "1.0.0"
}

func (l *LoggerPlugin) Init(config map[string]interface{}) error {
	l.config = config
	fmt.Printf("[Logger Plugin] Initialized with config: %v\n", config)
	return nil
}

func (l *LoggerPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	fmt.Printf("[Logger Plugin] %s Request to %s at %s\n",
		req.Method,
		req.Endpoint,
		time.Now().Format(time.RFC3339))

	// Add a custom header to track the plugin
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["X-Plugin-Logger"] = "enabled"

	// Continue with the request
	return req, true, nil
}

func (l *LoggerPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	fmt.Printf("[Logger Plugin] Response from %s: status=%d, size=%d bytes\n",
		req.Endpoint,
		resp.StatusCode,
		len(resp.Body))

	// Add metadata
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]string)
	}
	resp.Metadata["logged_at"] = time.Now().Format(time.RFC3339)

	return resp, nil
}

func (l *LoggerPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	fmt.Printf("[Logger Plugin] Cache HIT for %s %s\n",
		req.Method,
		req.Endpoint)
	return resp, nil
}

func (l *LoggerPlugin) Shutdown() error {
	fmt.Println("[Logger Plugin] Shutting down")
	return nil
}

func main() {
	// This is required for Go plugins, but won't be called when loaded as a plugin
}
