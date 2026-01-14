package plugin

import (
	"context"
	"fmt"
	"plugin"
	"sync"
)

// GoPlugin wraps a Go plugin loaded from a shared library
type GoPlugin struct {
	name    string
	version string
	plugin  *plugin.Plugin
	impl    Plugin
	mu      sync.RWMutex
}

// LoadGoPlugin loads a Go plugin from the specified path
func LoadGoPlugin(path string) (Plugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Look for the NewPlugin symbol
	symPlugin, err := p.Lookup("NewPlugin")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not export NewPlugin: %w", path, err)
	}

	// Assert that the symbol is a plugin factory function
	newPlugin, ok := symPlugin.(func() Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin %s NewPlugin has invalid signature", path)
	}

	// Create the plugin instance
	pluginImpl := newPlugin()

	return &GoPlugin{
		name:   pluginImpl.Name(),
		plugin: p,
		impl:   pluginImpl,
	}, nil
}

func (g *GoPlugin) Name() string {
	return g.impl.Name()
}

func (g *GoPlugin) Version() string {
	return g.impl.Version()
}

func (g *GoPlugin) Init(config map[string]interface{}) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.impl.Init(config)
}

func (g *GoPlugin) OnRequest(ctx context.Context, req *Request) (*Request, bool, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.impl.OnRequest(ctx, req)
}

func (g *GoPlugin) OnResponse(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.impl.OnResponse(ctx, req, resp)
}

func (g *GoPlugin) OnCacheHit(ctx context.Context, req *Request, resp *Response) (*Response, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.impl.OnCacheHit(ctx, req, resp)
}

func (g *GoPlugin) Shutdown() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.impl.Shutdown()
}
