# apiproxyd - On-Premises API Caching Daemon

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![Status](https://img.shields.io/badge/Status-Beta-orange)](https://github.com/afterdarksys/apiproxyd)

A local caching companion for [api.apiproxy.app](https://api.apiproxy.app). It can reduce repeated upstream calls and latency when workloads contain cacheable, identical requests.

> **Project status:** The SQLite-backed local proxy and CLI are usable and tested. PostgreSQL, clustering, queues, LDAP, and third-party plugins should be treated as optional beta features and validated in your environment before production use. Performance and savings depend on workload and have not yet been independently benchmarked. See [BUGS.md](BUGS.md) before deploying it.

The daemon binds to loopback by default. Remote binding is refused unless `security.allow_remote` is explicitly enabled; if enabled, place it behind an authenticating network boundary because the local control and proxy routes are not a complete multi-user authorization layer.

## Features

- **Local cache** - Memory and SQLite cache layers for repeated requests
- **Optional shared cache** - PostgreSQL support for deployments that need shared state
- **Offline reads** - Serve previously cached responses for configured endpoints
- **Local security controls** - Loopback-only default, endpoint allowlists, rate limits, and SSRF checks
- **Deployment** - Single Go binary and a non-root Docker image
- **Monitoring** - Health checks, cache statistics, and Prometheus metrics
- **Configuration** - JSON/YAML files, environment variables, and CLI flags
- **Experimental extensions** - Go/Python plugins, LDAP, queues, and clustering
- **LLM context store** - Optional coding-agent session memory, context packets, and exact response caching

## Compatibility and Scope

apiproxyd is a caching client and local proxy for `api.apiproxy.app`. It is
**not currently OpenAI API compatible**: it does not expose the standard
OpenAI `/v1` contract, stream Server-Sent Events, or faithfully preserve all
upstream status codes and headers. The `/llm` routes are a local context store,
not an OpenAI inference endpoint. The example OpenAI Python adapter is
experimental and is not supported as a compatibility layer.

## Quick Start

### Installation

```bash
# Using Go
go install github.com/afterdarksys/apiproxyd@latest

# Or clone and build
git clone https://github.com/afterdarksys/apiproxyd.git
cd apiproxyd
make build

# Or use the installer
python3 install.py
```

### Configuration

Create `config.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9002
  },
  "entry_point": "https://api.apiproxy.app",
  "api_key": "apx_live_your_key_here",
  "cache": {
    "backend": "sqlite",
    "path": "~/.apiproxy/cache.db",
    "ttl": 86400
  },
  "offline_endpoints": ["/darkapi/*", "/dnsscience/*"],
  "whitelisted_endpoints": ["/darkapi/*", "/dnsscience/*", "/v1/darkapi/*"]
}
```

Or copy the example:
```bash
cp config.json.example config.json
# Edit with your API key
```

### Usage

```bash
# Authenticate
apiproxy login --api-key apx_live_xxxxx

# Start daemon
apiproxy daemon start

# Make cached API requests
apiproxy api GET /darkapi/ip/8.8.8.8

# Or via HTTP proxy
curl http://localhost:9002/api/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxxxx"
```

## Use Cases

### 1. Cost Reduction
Repeated identical requests can be served locally. Savings are approximately the workload's cache-hit rate; measure this against your own traffic before estimating cost reduction.

### 2. Performance Improvement
Cache hits avoid the upstream network round trip. Actual latency depends on hardware, cache backend, response size, and middleware configuration.

### 3. Offline Capability
Configure critical endpoints to work offline using cached data.

```bash
# Designate offline endpoints in config.json
"offline_endpoints": [
  "/darkapi/*",
  "/dnsscience/*"
]

# Requests continue working even without internet
curl http://localhost:9002/api/darkapi/ip/8.8.8.8
# ✅ Returns cached response with X-Offline: true header
```

### 4. Multi-Server Deployments
Use PostgreSQL backend to share cache across multiple application servers.

```
┌──────┐  ┌──────┐  ┌──────┐
│ App1 │  │ App2 │  │ App3 │
│ +APD │  │ +APD │  │ +APD │
└───┬──┘  └───┬──┘  └───┬──┘
    │         │         │
    └─────────┴─────────┘
              │
       ┌──────▼──────┐
       │ PostgreSQL  │
       │(Shared Cache)│
       └─────────────┘
```

### 5. LLM Coding-Agent Context
Enable the optional LLM context store to keep repo/session context locally for tools such as Codex, Claude, and Gemini adapters.

```json
"llm_context": {
  "enabled": true,
  "path": "~/.apiproxy/llm_context.db",
  "max_request_bytes": 10485760,
  "default_packet_bytes": 12000
}
```

Store session events by repo/worktree identity, then build a compact task packet when model context gets tight:

```bash
curl http://localhost:9002/llm/sessions \
  -H "Content-Type: application/json" \
  -d '{"provider":"openai","working_dir":"/repo","git_remote":"git@example.com:org/repo","git_branch":"main"}'

curl http://localhost:9002/llm/events \
  -H "Content-Type: application/json" \
  -d '{"session_id":"SESSION_ID","kind":"decision","source":"design","content":"Store context by git repo and working directory."}'

curl http://localhost:9002/llm/packet \
  -H "Content-Type: application/json" \
  -d '{"session_id":"SESSION_ID","question":"What context should the model keep?","max_bytes":12000}'
```

## Architecture

```
Application
    ↓
apiproxyd (Local Proxy)
    ↓
Cache Check
    ├── HIT  → Return cached (5ms)
    └── MISS → Fetch from api.apiproxy.app (200ms)
                 ↓
            Cache response
                 ↓
            Return to application
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design.

## CLI Commands

### Authentication
```bash
apiproxy login                         # Interactive login
apiproxy login --api-key apx_live_xxx  # Login with API key
```

### Daemon Management
```bash
apiproxy daemon start     # Run the daemon in the foreground
apiproxy daemon stop      # Stop daemon
apiproxy daemon status    # Check daemon status
apiproxy daemon restart   # Restart daemon
```

`daemon start` currently runs in the foreground. Use your service manager (systemd, launchd, Docker, etc.) to supervise it.

### API Requests
```bash
apiproxy api GET /darkapi/ip/8.8.8.8
apiproxy api POST /v1/nerdapi/hash --data '{"value":"test"}'
apiproxy api GET /v1/status --no-cache      # Bypass cache
apiproxy api GET /v1/ip/1.1.1.1 --cache-only # Only from cache
```

### LLM Context API
The LLM context API is available only when `llm_context.enabled` is true.

```bash
POST /llm/sessions       # Create or update a repo/workdir session
POST /llm/events         # Append context, decisions, summaries, or tool output
POST /llm/packet         # Build a compact task packet from stored context
POST /llm/cache/lookup   # Exact request/response cache lookup
POST /llm/cache/store    # Store an exact LLM response cache entry
```

### Configuration
```bash
apiproxy config show                    # Display configuration
apiproxy config show --format json      # JSON output
apiproxy config set cache.ttl 3600      # Set cache TTL
apiproxy config init                    # Create default config
```

### Testing & Debugging
```bash
apiproxy test              # Run diagnostics
apiproxy test --verbose    # Detailed output
apiproxy console           # Interactive REPL
```

## Configuration Reference

### config.json Structure

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `server.host` | string | Listen address | `127.0.0.1` |
| `server.port` | int | Listen port | `9002` |
| `entry_point` | string | Upstream API URL | `https://api.apiproxy.app` |
| `api_key` | string | Your API key | (required) |
| `cache.backend` | string | `sqlite` or `postgres` | `sqlite` |
| `cache.path` | string | SQLite database path | `~/.apiproxy/cache.db` |
| `cache.ttl` | int | Cache TTL (seconds) | `86400` (24h) |
| `cache.postgres_dsn` | string | PostgreSQL connection string | - |
| `security.allow_remote` | bool | Permit non-loopback binding (requires an external auth boundary) | `false` |
| `llm_context.enabled` | bool | Enable local LLM context endpoints | `false` |
| `llm_context.path` | string | SQLite database path for LLM context | `~/.apiproxy/llm_context.db` |
| `llm_context.max_request_bytes` | int | Max request body size for LLM context endpoints | `10485760` |
| `llm_context.default_packet_bytes` | int | Default context packet byte budget | `12000` |
| `offline_endpoints` | array | Endpoints that work offline | `[]` |
| `whitelisted_endpoints` | array | Allowed endpoints | `[]` |

See [config.json.example](config.json.example) for complete example.

## Deployment

### Development
```bash
make build
./apiproxy daemon start
```

### Docker

For a published container port, set `server.host` to `0.0.0.0` and
`security.allow_remote` to `true` in the mounted config. Do this only behind an
authenticating network boundary.

```bash
# Build image
make docker-build

# Run container
docker run -p 9002:9002 \
  -v $(pwd)/config.json:/app/config.json:ro \
  apiproxyd:latest
```

See [DEPLOYMENT.md](DEPLOYMENT.md) for complete deployment guide.

## Plugin System

apiproxyd includes an experimental plugin system. Plugins can intercept and
modify requests and responses, but the examples have not been validated as
production integrations.

### Plugin Types

- **Go Plugins** - Compiled as shared libraries (.so), high performance, loaded in-process
- **Python Plugins** - Executed as subprocesses, easy to develop, slower than Go

### Plugin Use Cases

The plugin interfaces can be used to prototype:

- **Custom routing** - Route selected paths to another service
- **Authentication hooks** - Add or normalize upstream credentials
- **Data transformation** - Modify requests and responses
- **Logging and metrics** - Observe plugin-handled traffic

### Quick Example

Add plugins to your `config.json`:

```json
{
  "plugins": {
    "enabled": true,
    "plugins": [
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
      }
    ]
  }
}
```

Plugin examples are starting points and require integration testing before deployment.

### Example Plugins

The repository includes experimental examples:

1. **Infoblox Plugin** (Go) - Complete NIOS/WAPI API caching with intelligent TTLs
   - Network objects: 1h, DNS records: 5m, DHCP: 2m, Grid config: 30m
   - Identity-independent caching, automatic mutation detection
2. **BlueCat Plugin** (Go) - Address Manager (BAM/BDDS) API caching
   - Config objects: 1h, DNS: 5m, DHCP: 2m, Searches: 10m, Deployment: 30s
   - Full support for BAM REST API operations
3. **LDAP Authentication** (Go Middleware) - Enterprise directory integration
   - Connection pooling, TLS/SSL, group membership validation
   - Authentication result caching for performance
4. **Logger Plugin** (Go/Python) - Logs all requests and responses
5. **Custom Router** (Go) - Routes requests to external APIs by pattern
6. **OpenAI Adapter** (Python) - Incomplete proof of concept; not OpenAI API compatible
7. **Web Admin UI** (Go) - Real-time debugging dashboard on port 9003
8. **AWS SigV4 Proxy** (Go) - Strips signatures for identity-aware proxying
9. **Docker Registry** (Go) - Intercepts manifest/blob fetches for OCI caching
10. **Cloudflare ETag** (Go) - Automatic If-None-Match tracking and TTL updating
11. **Vercel API** (Go) - Query normalization for Analytics APIs
12. **Dartnode API** (Go) - Identity token normalization for cache sharing
13. **Kubernetes Cache** (Go) - In-memory materialized views for Kubernetes resources

### Building Plugins

```bash
# Build all example plugins
cd examples/plugins
make all

# Install to system location
make install

# Build Infoblox plugin
cd examples/plugins/go/infoblox
go build -buildmode=plugin -o infoblox.so infoblox.go

# Build BlueCat plugin
cd examples/plugins/go/bluecat
go build -buildmode=plugin -o bluecat.so bluecat.go
```

See [PLUGINS_README.md](PLUGINS_README.md) for comprehensive plugin documentation, development guide, and configuration examples.

## Performance

No reproducible benchmark suite is checked in yet. Treat latency, throughput, and cost-reduction figures as workload-dependent until benchmarks are added.

## Security

### API Key Storage
- Stored in config file with `chmod 600`
- Never logged or exposed in responses
- Support for environment variables

### Network Security
- Binds to `127.0.0.1` by default (local-only)
- Whitelisted endpoints prevent unauthorized access
- HTTPS for all upstream requests

### Cache Security
- File permissions: `600` (owner only)
- PostgreSQL with strong passwords
- SSL/TLS support for PostgreSQL

## Monitoring

### Health Check
```bash
curl http://localhost:9002/health
```

Response:
```json
{
  "status": "ok",
  "version": "0.3.0",
  "uptime": 3600.5
}
```

### Cache Statistics
```bash
curl http://localhost:9002/cache/stats
```

Response:
```json
{
  "entries": 1234,
  "size_bytes": 567890,
  "hit_rate": 0.85,
  "hits": 10000,
  "misses": 1500
}
```

### Prometheus Metrics
```bash
curl http://localhost:9002/metrics
```

Exports metrics in Prometheus format:
- `apiproxyd_requests_total` - Total requests processed
- `apiproxyd_cache_hits_total` - Cache hit count
- `apiproxyd_cache_misses_total` - Cache miss count
- `apiproxyd_bytes_transferred_total` - Total bytes transferred
- `apiproxyd_requests_by_method` - Requests grouped by HTTP method
- `apiproxyd_requests_by_status` - Requests grouped by status code

### Web Admin UI
Enable the web admin plugin to access a real-time debugging interface:

```bash
# Visit http://localhost:9003 after enabling the plugin
```

Features:
- Real-time request/response inspection
- Live statistics dashboard
- Cache hit/miss tracking
- Response time monitoring

## Troubleshooting

### Daemon won't start
```bash
# Check if port is in use
lsof -i :9002

# Run in foreground to see errors
apiproxy daemon start --foreground

# Check configuration
apiproxy test
```

### Cache not working
```bash
# View cache stats
curl http://localhost:9002/cache/stats

# Clear cache
curl -X POST http://localhost:9002/cache/clear

# Check disk space
df -h ~/.apiproxy/
```

### Authentication failures
```bash
# Re-authenticate
apiproxy login

# Verify API key
apiproxy config show

# Test upstream connectivity
curl https://api.apiproxy.app/v1/validate \
  -H "X-API-Key: apx_live_xxx"
```

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**:
  - [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture and design
  - [DEPLOYMENT.md](DEPLOYMENT.md) - Production deployment guide
  - [INSTALL.md](INSTALL.md) - Installation instructions
  - [PLUGINS_README.md](PLUGINS_README.md) - Plugin development guide
  - [CHANGELOG.md](CHANGELOG.md) - Version history
  - [IMPLEMENTATION_SUMMARY_2026.md](IMPLEMENTATION_SUMMARY_2026.md) - Complete feature summary
- **Issues**: [GitHub Issues](https://github.com/afterdarksys/apiproxyd/issues)
- **Main Site**: [api.apiproxy.app](https://api.apiproxy.app)
- **Email Support**: support@apiproxy.app

## Roadmap

- [x] Plugin system (Go and Python)
- [x] Custom API routing via plugins
- [x] Prometheus metrics exporter
- [x] Response compression (gzip)
- [x] Web admin UI for debugging
- [x] Infoblox NIOS/WAPI integration
- [x] BlueCat Address Manager integration
- [x] LDAP/Active Directory authentication
- [x] gRPC distributed caching
- [ ] Grafana dashboard templates
- [ ] Kubernetes Helm charts
- [ ] Cache warming functionality (partial)
- [ ] Intelligent TTL adjustment (plugin-based)
- [ ] Multi-tenancy support
- [ ] Plugin marketplace/registry

## Related Projects

- [api.apiproxy.app](https://github.com/afterdarktech/apiproxy.app) - Main API gateway service
- [darkapi.io](https://darkapi.io) - IP intelligence API
- [nerdapi.io](https://nerdapi.io) - Developer utilities API

---

**Made with ❤️ by After Dark Systems, LLC**
