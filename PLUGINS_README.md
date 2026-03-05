# apiproxyd Plugins Documentation

## Overview

apiproxyd includes a powerful plugin system that allows you to extend its functionality with custom API integrations, authentication methods, and caching strategies. This document covers the official plugins and how to use them.

---

## Plugin System Architecture

### Plugin Types

apiproxyd supports two types of plugins:

1. **Go Plugins** (`.so` files) - High-performance native plugins
2. **Python Plugins** (`.py` files) - Flexible, easy-to-develop plugins

### Plugin Lifecycle Hooks

All plugins implement the following hooks:

- `OnRequest` - Called before proxying the request (request modification, authentication)
- `OnResponse` - Called after receiving upstream response (response modification, caching control)
- `OnCacheHit` - Called when a cached response is found
- `Shutdown` - Called during graceful shutdown

---

## Official Plugins

### 1. Infoblox Plugin

**Purpose:** Optimizes caching for Infoblox NIOS/WAPI API calls (DNS, DHCP, IPAM).

**Location:** `examples/plugins/go/infoblox/infoblox.go`

**Features:**
- Intelligent cache TTL based on object type:
  - Network objects (IP blocks, subnets): 1 hour
  - DNS records: 5 minutes
  - DHCP leases: 2 minutes
  - Grid/config: 30 minutes
  - Search operations: 1 minute
- Identity-independent caching (strips auth cookies)
- No caching for mutations (POST/PUT/DELETE)

**Supported Endpoints:**
- `/wapi/v*/network*` - Network and subnet queries
- `/wapi/v*/record:*` - DNS record lookups
- `/wapi/v*/lease` - DHCP lease queries
- `/wapi/v*/grid` - Grid configuration
- `/wapi/v*/search` - Search operations

**Configuration Example:**
```json
{
  "plugins": {
    "enabled": true,
    "plugins": [
      {
        "name": "infoblox",
        "type": "go",
        "path": "./examples/plugins/go/infoblox/infoblox.so",
        "enabled": true,
        "config": {
          "cache_network_objects": 3600,
          "cache_dns_records": 300,
          "cache_dhcp_objects": 120
        }
      }
    ]
  }
}
```

**Building:**
```bash
cd examples/plugins/go/infoblox
go build -buildmode=plugin -o infoblox.so infoblox.go
```

---

### 2. BlueCat Plugin

**Purpose:** Optimizes caching for BlueCat Address Manager (BAM) and BDDS REST API.

**Location:** `examples/plugins/go/bluecat/bluecat.go`

**Features:**
- Intelligent cache TTL based on API endpoint:
  - Configuration objects (IP blocks, networks): 1 hour
  - DNS zones and records: 5 minutes
  - DHCP ranges and reservations: 2 minutes
  - Entity searches: 10 minutes
  - Deployment status: 30 seconds
  - System info: 1 hour
- Mutation detection (add*/update*/delete*/deploy*)
- Identity-independent caching

**Supported Endpoints:**
- `/Services/REST/v1/getIP*` - IP and network queries
- `/Services/REST/v1/*Record` - DNS record operations
- `/Services/REST/v1/getDHCP*` - DHCP operations
- `/Services/REST/v1/search*` - Search operations
- `/Services/REST/v1/getDeployment*` - Deployment status

**Configuration Example:**
```json
{
  "plugins": {
    "enabled": true,
    "plugins": [
      {
        "name": "bluecat",
        "type": "go",
        "path": "./examples/plugins/go/bluecat/bluecat.so",
        "enabled": true,
        "config": {
          "cache_config_objects": 3600,
          "cache_dns_records": 300,
          "cache_dhcp_objects": 120,
          "cache_searches": 600
        }
      }
    ]
  }
}
```

**Building:**
```bash
cd examples/plugins/go/bluecat
go build -buildmode=plugin -o bluecat.so bluecat.go
```

---

### 3. LDAP Authentication Middleware

**Purpose:** Adds LDAP/Active Directory authentication to apiproxyd.

**Location:** `pkg/middleware/ldap.go`

**Features:**
- Connection pooling for high performance
- TLS/SSL support (LDAPS or StartTLS)
- Group membership validation
- Authentication result caching
- Automatic connection health checks
- Graceful connection lifecycle management

**Configuration Example:**
```json
{
  "ldap": {
    "enabled": true,
    "server": "ldap.company.com:636",
    "use_tls": true,
    "use_ssl": false,
    "insecure": false,
    "bind_dn": "cn=apiproxy-service,ou=services,dc=company,dc=com",
    "bind_password": "service_account_password",
    "base_dn": "ou=users,dc=company,dc=com",
    "user_filter": "(uid=%s)",
    "require_group": "cn=api-users,ou=groups,dc=company,dc=com",
    "pool_size": 10,
    "conn_max_lifetime": 600,
    "conn_timeout": 10,
    "cache_enabled": true,
    "cache_ttl": 300
  }
}
```

**Usage:**
```bash
# Client authenticates with Basic Auth
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -u username:password
```

**LDAP Configuration Parameters:**

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `enabled` | bool | Enable LDAP authentication | false |
| `server` | string | LDAP server address (host:port) | - |
| `use_tls` | bool | Use direct TLS (LDAPS, port 636) | false |
| `use_ssl` | bool | Use StartTLS upgrade (port 389) | false |
| `insecure` | bool | Skip TLS certificate verification | false |
| `bind_dn` | string | Service account DN | - |
| `bind_password` | string | Service account password | - |
| `base_dn` | string | Base DN for user searches | - |
| `user_filter` | string | LDAP filter for user search | `(uid=%s)` |
| `require_group` | string | Required group membership DN | "" |
| `pool_size` | int | Connection pool size | 10 |
| `conn_max_lifetime` | int | Max connection lifetime (seconds) | 600 |
| `conn_timeout` | int | Connection timeout (seconds) | 10 |
| `cache_enabled` | bool | Enable auth result caching | false |
| `cache_ttl` | int | Cache TTL (seconds) | 300 |

---

### 4. Docker Registry Plugin

**Purpose:** Optimizes caching for OCI/Docker registry APIs.

**Location:** `examples/plugins/go/docker_registry/docker_registry.go`

**Features:**
- Immutable blob caching (sha256-based)
- Manifest caching with short TTL
- Identity-independent caching

---

### 5. AWS SigV4 Plugin

**Purpose:** Caches AWS API responses with signature stripping.

**Location:** `examples/plugins/go/aws_sigv4/aws_sigv4.go`

**Features:**
- Strips AWS SigV4 authentication headers for identity-independent caching
- Preserves request integrity
- Compatible with all AWS services

---

### 6. Kubernetes Cache Plugin

**Purpose:** In-memory materialized views for Kubernetes resources.

**Location:** `examples/plugins/go/kubernetes_cache/kubernetes_cache.go`

**Features:**
- High-performance in-memory caching
- Resource-specific TTLs
- Watch-based cache invalidation

---

### 7. OpenAI Adapter Plugin

**Purpose:** OpenAI API integration with cost tracking.

**Location:** `examples/plugins/python/openai_adapter.py`

**Features:**
- Request/response logging
- Token usage tracking
- Cost calculation
- Rate limit handling

---

## Creating Custom Plugins

### Go Plugin Template

```go
package main

import (
    "context"
    "github.com/afterdarksys/apiproxyd/pkg/plugin"
)

type MyPlugin struct {
    config map[string]interface{}
}

func NewPlugin() plugin.Plugin {
    return &MyPlugin{}
}

func (p *MyPlugin) Name() string    { return "my-plugin" }
func (p *MyPlugin) Version() string { return "1.0.0" }

func (p *MyPlugin) Init(config map[string]interface{}) error {
    p.config = config
    return nil
}

func (p *MyPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
    // Modify request here
    return req, true, nil
}

func (p *MyPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    // Modify response here
    return resp, nil
}

func (p *MyPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
    return resp, nil
}

func (p *MyPlugin) Shutdown() error {
    return nil
}

func main() {}
```

### Python Plugin Template

```python
class MyPlugin:
    def __init__(self):
        self.config = {}

    def name(self):
        return "my-plugin"

    def version(self):
        return "1.0.0"

    def init(self, config):
        self.config = config

    def on_request(self, ctx, req):
        # Modify request
        return req, True, None

    def on_response(self, ctx, req, resp):
        # Modify response
        return resp, None

    def on_cache_hit(self, ctx, req, resp):
        return resp, None

    def shutdown(self):
        pass
```

---

## Plugin Development Best Practices

1. **Keep plugins focused** - One plugin should handle one concern
2. **Minimize memory usage** - Avoid storing large state in plugins
3. **Handle errors gracefully** - Return errors instead of panicking
4. **Use structured logging** - Include plugin name in log messages
5. **Test thoroughly** - Write unit tests for plugin logic
6. **Document configuration** - Provide clear examples and defaults
7. **Version your plugins** - Use semantic versioning

---

## Troubleshooting

### Plugin Not Loading

**Problem:** Plugin fails to load with "symbol not found" error

**Solution:**
- Ensure plugin is built with same Go version as apiproxyd
- Verify plugin implements all required interface methods
- Check plugin path in configuration

### Plugin Crashes

**Problem:** apiproxyd crashes when plugin is enabled

**Solution:**
- Check for nil pointer dereferences in plugin code
- Ensure proper error handling
- Review plugin logs for panic messages
- Build plugin with race detector: `go build -race -buildmode=plugin`

### Performance Issues

**Problem:** Requests are slow when plugin is enabled

**Solution:**
- Profile plugin with pprof
- Minimize blocking operations in hooks
- Use caching within plugin if appropriate
- Consider moving heavy processing to background goroutines

---

## Plugin Compatibility Matrix

| Plugin | apiproxyd Version | Go Version | Status |
|--------|-------------------|------------|--------|
| Infoblox | v0.2.0+ | 1.25+ | Production |
| BlueCat | v0.2.0+ | 1.25+ | Production |
| LDAP Auth | v0.2.0+ | 1.25+ | Production |
| Docker Registry | v0.2.0+ | 1.25+ | Production |
| AWS SigV4 | v0.2.0+ | 1.25+ | Production |
| Kubernetes | v0.2.0+ | 1.25+ | Beta |
| OpenAI | v0.2.0+ | Python 3.9+ | Production |

---

## License

All plugins are licensed under the same license as apiproxyd.

---

## Contributing

We welcome contributions! To contribute a plugin:

1. Fork the repository
2. Create a new plugin in `examples/plugins/go/<plugin-name>/` or `examples/plugins/python/<plugin-name>/`
3. Add comprehensive documentation
4. Write tests
5. Submit a pull request

For questions or support, open an issue on GitHub or contact support@apiproxy.app.
