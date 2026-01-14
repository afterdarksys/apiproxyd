# apiproxyd Plugins

This directory contains example plugins for apiproxyd. Plugins allow you to extend apiproxyd's functionality by intercepting and modifying requests and responses at various stages of the proxy pipeline.

## Plugin System Overview

The plugin system supports two types of plugins:

1. **Go Plugins** - Compiled as shared libraries (.so files) and loaded dynamically
2. **Python Plugins** - Executed as subprocesses with JSON-RPC communication

## Plugin Lifecycle Hooks

Plugins can implement the following hooks:

- **OnRequest** - Called before proxying a request (can modify request or stop processing)
- **OnResponse** - Called after receiving upstream response (can modify response)
- **OnCacheHit** - Called when a cached response is found (can modify cached response)

## Use Cases

Plugins enable powerful integrations:

- ✅ Route requests to custom APIs (Stripe, Twilio, AWS, etc.)
- ✅ Add authentication and API key management
- ✅ Transform request/response formats
- ✅ Implement rate limiting and quotas
- ✅ Add logging and monitoring
- ✅ Inject custom headers or metadata
- ✅ Cache third-party API responses
- ✅ Cost tracking and billing

## Example Plugins

### Go Plugins

#### 1. Logger Plugin (`go/logger/`)
Simple logging plugin that logs all requests and responses.

**Features:**
- Logs request method, endpoint, and timestamp
- Logs response status and size
- Adds custom headers to track plugin execution

**Build:**
```bash
cd go/logger
go build -buildmode=plugin -o logger.so logger.go
```

#### 2. Custom Router Plugin (`go/custom_router/`)
Routes specific endpoints to custom APIs based on patterns.

**Features:**
- Pattern-based routing (e.g., `/v1/custom/*` → `https://my-api.com`)
- Forwards requests to external APIs
- Configurable route mappings
- Perfect for integrating third-party services

**Build:**
```bash
cd go/custom_router
go build -buildmode=plugin -o custom_router.so router.go
```

**Configuration:**
```json
{
  "routes": {
    "/v1/stripe/*": "https://api.stripe.com",
    "/v1/twilio/*": "https://api.twilio.com",
    "/v1/custom/*": "https://my-internal-api.com"
  }
}
```

### Python Plugins

#### 1. Logger Plugin (`python/logger.py`)
Python version of the logger plugin with the same functionality as the Go version.

**Features:**
- Logs requests and responses
- Adds custom metadata
- Demonstrates JSON-RPC communication

**Usage:**
```bash
chmod +x python/logger.py
# Plugin will be executed automatically by apiproxyd
```

#### 2. OpenAI Adapter Plugin (`python/openai_adapter.py`)
Adapts OpenAI API requests for caching and monitoring through apiproxyd.

**Features:**
- Transforms `/v1/openai/*` endpoints to OpenAI API
- Adds authentication headers
- Extracts and logs token usage
- Adds cost tracking metadata
- Perfect for reducing OpenAI API costs with caching

**Configuration:**
```json
{
  "openai_api_key": "sk-..."
}
```

## Building Plugins

### Build All Plugins
```bash
make all
```

### Build Only Go Plugins
```bash
make go
```

### Setup Python Plugins
```bash
make python
```

### Install Plugins
```bash
make install
```

This installs plugins to `~/.apiproxy/plugins/`.

### Clean Build Artifacts
```bash
make clean
```

## Configuring Plugins

Add plugin configuration to your `config.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9002
  },
  "entry_point": "https://api.apiproxy.app",
  "api_key": "apx_live_xxxxx",
  "cache": {
    "backend": "sqlite",
    "path": "~/.apiproxy/cache.db",
    "ttl": 86400
  },
  "plugins": {
    "enabled": true,
    "plugins": [
      {
        "name": "logger",
        "type": "go",
        "path": "~/.apiproxy/plugins/go/logger.so",
        "enabled": true,
        "config": {}
      },
      {
        "name": "custom_router",
        "type": "go",
        "path": "~/.apiproxy/plugins/go/custom_router.so",
        "enabled": true,
        "config": {
          "routes": {
            "/v1/stripe/*": "https://api.stripe.com",
            "/v1/openai/*": "https://api.openai.com"
          }
        }
      },
      {
        "name": "openai_adapter",
        "type": "python",
        "path": "~/.apiproxy/plugins/python/openai_adapter.py",
        "enabled": true,
        "config": {
          "openai_api_key": "sk-..."
        }
      }
    ]
  }
}
```

## Writing Your Own Plugins

### Go Plugin Structure

```go
package main

import (
    "context"
    "github.com/afterdarksys/apiproxyd/pkg/plugin"
)

type MyPlugin struct {
    config map[string]interface{}
}

// Required factory function
func NewPlugin() plugin.Plugin {
    return &MyPlugin{}
}

func (p *MyPlugin) Name() string {
    return "my_plugin"
}

func (p *MyPlugin) Version() string {
    return "1.0.0"
}

func (p *MyPlugin) Init(config map[string]interface{}) error {
    p.config = config
    return nil
}

func (p *MyPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
    // Modify request or return (req, false, nil) to stop processing
    return req, true, nil
}

func (p *MyPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    // Modify response
    return resp, nil
}

func (p *MyPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    // Handle cache hits
    return resp, nil
}

func (p *MyPlugin) Shutdown() error {
    return nil
}

func main() {}
```

**Build:**
```bash
go build -buildmode=plugin -o my_plugin.so my_plugin.go
```

### Python Plugin Structure

```python
#!/usr/bin/env python3
import json
import sys

class MyPlugin:
    def __init__(self):
        self.config = {}

    def get_info(self):
        return {
            "name": "my_plugin",
            "version": "1.0.0"
        }

    def init(self, config):
        self.config = config
        return {"status": "ok"}

    def on_request(self, request_json):
        request = json.loads(request_json)
        # Modify request
        return {
            "request": request,
            "continue": True  # False to stop processing
        }

    def on_response(self, request_json, response_json):
        response = json.loads(response_json)
        # Modify response
        return response

    def on_cache_hit(self, request_json, response_json):
        response = json.loads(response_json)
        # Handle cache hit
        return response

    def shutdown(self):
        return {"status": "ok"}

# JSON-RPC handler (copy from examples)
# ... rest of the boilerplate code
```

**Make executable:**
```bash
chmod +x my_plugin.py
```

## Plugin Request/Response Format

### Request
```json
{
  "method": "GET",
  "endpoint": "/v1/api/endpoint",
  "headers": {
    "Authorization": "Bearer xxx",
    "Content-Type": "application/json"
  },
  "body": "...",
  "metadata": {}
}
```

### Response
```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "...",
  "cached": false,
  "metadata": {}
}
```

## Advanced Examples

### Rate Limiting Plugin

```go
// Track requests per API key and enforce rate limits
func (p *RateLimitPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
    apiKey := req.Headers["X-API-Key"]
    if p.exceedsRateLimit(apiKey) {
        return req, false, fmt.Errorf("rate limit exceeded")
    }
    p.trackRequest(apiKey)
    return req, true, nil
}
```

### Response Transformation Plugin

```go
// Transform response format (e.g., XML to JSON)
func (p *TransformPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    if strings.Contains(req.Endpoint, "/xml/") {
        jsonData := convertXMLToJSON(resp.Body)
        resp.Body = jsonData
        resp.Headers["Content-Type"] = "application/json"
    }
    return resp, nil
}
```

### Cost Tracking Plugin

```go
// Track API costs per request
func (p *CostTrackerPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    cost := p.calculateCost(req, resp)
    p.recordCost(req.Headers["X-API-Key"], cost)
    resp.Metadata["cost"] = fmt.Sprintf("%.4f", cost)
    return resp, nil
}
```

## Troubleshooting

### Go Plugin Issues

**Error: plugin was built with a different version of package X**
- Rebuild the plugin with the same Go version as apiproxyd
- Ensure all dependencies match

**Error: plugin.Open: plugin not found**
- Check the plugin path in config.json
- Ensure the .so file exists and is readable

### Python Plugin Issues

**Error: failed to start plugin process**
- Ensure Python 3 is installed
- Make the plugin executable: `chmod +x plugin.py`
- Check the shebang line: `#!/usr/bin/env python3`

**Error: plugin closed connection**
- Check stderr logs for Python errors
- Ensure JSON-RPC protocol is correctly implemented

## Performance Considerations

- **Go plugins** are faster (loaded in-process, minimal overhead)
- **Python plugins** have higher overhead (subprocess communication) but are easier to develop
- Plugins are executed synchronously in the request pipeline
- Keep plugin logic lightweight for best performance

## Security

- Plugins run with the same privileges as apiproxyd
- Be cautious about loading untrusted plugins
- Validate and sanitize all plugin inputs
- Use secure communication for external API calls
- Store API keys securely in plugin config

## Contributing

Have a cool plugin idea? Contributions are welcome! Please submit a PR with:
- Plugin source code
- Documentation
- Example configuration
- Build instructions

## License

All example plugins are MIT licensed.
