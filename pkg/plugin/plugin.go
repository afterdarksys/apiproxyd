package plugin

import (
	"context"
	"encoding/json"
	"net/http"
)

// Plugin represents the interface that all plugins must implement
type Plugin interface {
	// Name returns the plugin name
	Name() string

	// Version returns the plugin version
	Version() string

	// Init initializes the plugin with configuration
	Init(config map[string]interface{}) error

	// OnRequest is called before proxying the request
	// Returns modified context, request data, and continue flag
	OnRequest(ctx context.Context, req *Request) (*Request, bool, error)

	// OnResponse is called after receiving the upstream response
	// Returns modified response data
	OnResponse(ctx context.Context, req *Request, resp *Response) (*Response, error)

	// OnCacheHit is called when a cached response is found
	OnCacheHit(ctx context.Context, req *Request, resp *Response) (*Response, error)

	// Shutdown gracefully shuts down the plugin
	Shutdown() error
}

// Request represents an API request
type Request struct {
	Method   string            `json:"method"`
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
	Body     []byte            `json:"body"`
	Metadata map[string]string `json:"metadata"` // Plugin-specific metadata
}

// Response represents an API response
type Response struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Cached     bool              `json:"cached"`
	Metadata   map[string]string `json:"metadata"` // Plugin-specific metadata
}

// Manager manages all loaded plugins
type Manager struct {
	plugins []Plugin
	config  *Config
}

// Config holds plugin configuration
type Config struct {
	Enabled bool            `json:"enabled" yaml:"enabled"`
	Plugins []PluginConfig  `json:"plugins" yaml:"plugins"`
}

// PluginConfig holds configuration for a single plugin
type PluginConfig struct {
	Name    string                 `json:"name" yaml:"name"`
	Type    string                 `json:"type" yaml:"type"` // "go" or "python"
	Path    string                 `json:"path" yaml:"path"`
	Enabled bool                   `json:"enabled" yaml:"enabled"`
	Config  map[string]interface{} `json:"config" yaml:"config"`
}

// NewManager creates a new plugin manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = &Config{Enabled: false}
	}
	return &Manager{
		plugins: make([]Plugin, 0),
		config:  config,
	}
}

// LoadPlugins loads all configured plugins
func (m *Manager) LoadPlugins() error {
	if !m.config.Enabled {
		return nil
	}

	for _, pc := range m.config.Plugins {
		if !pc.Enabled {
			continue
		}

		var plugin Plugin
		var err error

		switch pc.Type {
		case "go":
			plugin, err = LoadGoPlugin(pc.Path)
		case "python":
			plugin, err = LoadPythonPlugin(pc.Path, pc.Config)
		default:
			continue
		}

		if err != nil {
			return err
		}

		if err := plugin.Init(pc.Config); err != nil {
			return err
		}

		m.plugins = append(m.plugins, plugin)
	}

	return nil
}

// OnRequest executes all plugin OnRequest hooks
func (m *Manager) OnRequest(ctx context.Context, req *Request) (*Request, bool, error) {
	for _, plugin := range m.plugins {
		modifiedReq, cont, err := plugin.OnRequest(ctx, req)
		if err != nil {
			return req, false, err
		}
		if !cont {
			return modifiedReq, false, nil
		}
		req = modifiedReq
	}
	return req, true, nil
}

// OnResponse executes all plugin OnResponse hooks
func (m *Manager) OnResponse(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	for _, plugin := range m.plugins {
		modifiedResp, err := plugin.OnResponse(ctx, req, resp)
		if err != nil {
			return resp, err
		}
		resp = modifiedResp
	}
	return resp, nil
}

// OnCacheHit executes all plugin OnCacheHit hooks
func (m *Manager) OnCacheHit(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	for _, plugin := range m.plugins {
		modifiedResp, err := plugin.OnCacheHit(ctx, req, resp)
		if err != nil {
			return resp, err
		}
		resp = modifiedResp
	}
	return resp, nil
}

// Shutdown gracefully shuts down all plugins
func (m *Manager) Shutdown() error {
	for _, plugin := range m.plugins {
		if err := plugin.Shutdown(); err != nil {
			return err
		}
	}
	return nil
}

// FromHTTPRequest converts an http.Request to plugin.Request
func FromHTTPRequest(r *http.Request, body []byte) *Request {
	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &Request{
		Method:   r.Method,
		Endpoint: r.URL.Path,
		Headers:  headers,
		Body:     body,
		Metadata: make(map[string]string),
	}
}

// ToJSON serializes the request to JSON
func (r *Request) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// FromJSON deserializes the request from JSON
func (r *Request) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}

// ToJSON serializes the response to JSON
func (r *Response) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// FromJSON deserializes the response from JSON
func (r *Response) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}
