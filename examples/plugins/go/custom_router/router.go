package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// CustomRouterPlugin routes requests to custom APIs based on endpoint patterns
type CustomRouterPlugin struct {
	routes map[string]string // endpoint pattern -> custom API URL
	client *http.Client
}

// NewPlugin is the required factory function for Go plugins
func NewPlugin() plugin.Plugin {
	return &CustomRouterPlugin{
		routes: make(map[string]string),
		client: &http.Client{},
	}
}

func (c *CustomRouterPlugin) Name() string {
	return "custom_router"
}

func (c *CustomRouterPlugin) Version() string {
	return "1.0.0"
}

func (c *CustomRouterPlugin) Init(config map[string]interface{}) error {
	// Load routes from config
	// Example config:
	// {
	//   "routes": {
	//     "/v1/custom/*": "https://my-api.example.com",
	//     "/v1/external/weather": "https://api.weather.com"
	//   }
	// }
	if routesRaw, ok := config["routes"].(map[string]interface{}); ok {
		for pattern, url := range routesRaw {
			if urlStr, ok := url.(string); ok {
				c.routes[pattern] = urlStr
				fmt.Printf("[CustomRouter] Registered route: %s -> %s\n", pattern, urlStr)
			}
		}
	}
	return nil
}

func (c *CustomRouterPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Check if this endpoint matches any custom routes
	for pattern, baseURL := range c.routes {
		if c.matchPattern(pattern, req.Endpoint) {
			// Route to custom API
			fmt.Printf("[CustomRouter] Routing %s to custom API: %s\n", req.Endpoint, baseURL)

			// Build the full URL
			endpoint := strings.TrimPrefix(req.Endpoint, strings.TrimSuffix(pattern, "*"))
			fullURL := baseURL + endpoint

			// Make request to custom API
			httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, strings.NewReader(string(req.Body)))
			if err != nil {
				return req, false, fmt.Errorf("failed to create request: %w", err)
			}

			// Copy headers
			for k, v := range req.Headers {
				httpReq.Header.Set(k, v)
			}

			resp, err := c.client.Do(httpReq)
			if err != nil {
				return req, false, fmt.Errorf("failed to call custom API: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return req, false, fmt.Errorf("failed to read response: %w", err)
			}

			// Store the custom response in metadata so we can return it
			if req.Metadata == nil {
				req.Metadata = make(map[string]string)
			}
			req.Metadata["custom_response"] = string(body)
			req.Metadata["custom_status"] = fmt.Sprintf("%d", resp.StatusCode)
			req.Metadata["routed"] = "true"

			// Stop further processing - we handled this request
			return req, false, nil
		}
	}

	// No custom route matched, continue with normal processing
	return req, true, nil
}

func (c *CustomRouterPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// If we routed this request to a custom API, use that response instead
	if req.Metadata != nil && req.Metadata["routed"] == "true" {
		customBody := []byte(req.Metadata["custom_response"])

		return &plugin.Response{
			StatusCode: resp.StatusCode,
			Headers:    resp.Headers,
			Body:       customBody,
			Cached:     false,
			Metadata: map[string]string{
				"custom_api": "true",
			},
		}, nil
	}

	return resp, nil
}

func (c *CustomRouterPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// No modification needed for cache hits
	return resp, nil
}

func (c *CustomRouterPlugin) Shutdown() error {
	fmt.Println("[CustomRouter] Shutting down")
	c.client.CloseIdleConnections()
	return nil
}

func (c *CustomRouterPlugin) matchPattern(pattern, path string) bool {
	if pattern == path {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, prefix)
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return false
}

func main() {
	// Required for Go plugins
	plugin := NewPlugin()
	data, _ := json.MarshalIndent(map[string]string{
		"name":    plugin.Name(),
		"version": plugin.Version(),
	}, "", "  ")
	fmt.Println(string(data))
}
