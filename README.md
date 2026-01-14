# apiproxyd - On-Premises API Caching Daemon

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![Status](https://img.shields.io/badge/Status-Production%20Ready-green)](https://github.com/afterdarktech/apiproxyd)

A high-performance API caching daemon that enables businesses to deploy on-premises caching infrastructure for [api.apiproxy.app](https://api.apiproxy.app). Reduce API costs by up to 90% and improve response times from 200ms to under 5ms.

## Features

- 🚀 **High Performance** - Built in Go, handles 10K-100K cached requests/sec
- 💾 **Dual Cache Backends** - SQLite for single-server, PostgreSQL for multi-server deployments
- 🔒 **Secure** - API key authentication, whitelisted endpoints, encrypted storage
- 📴 **Offline Mode** - Continue serving cached responses without internet connectivity
- 🛠️ **Easy Deployment** - Single binary, Docker support, systemd integration
- 📊 **Monitoring** - Built-in health checks, cache statistics, and metrics
- 🔧 **Flexible Configuration** - JSON/YAML config, environment variables, CLI flags

## Quick Start

### Installation

```bash
# Using Go
go install github.com/afterdarktech/apiproxyd@latest

# Or clone and build
git clone https://github.com/afterdarktech/apiproxyd.git
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
  "offline_endpoints": ["/v1/darkapi/ip/*", "/health"],
  "whitelisted_endpoints": ["/v1/darkapi/*", "/v1/nerdapi/*"]
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
apiproxy api GET /v1/darkapi/ip/8.8.8.8

# Or via HTTP proxy
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxxxx"
```

## Use Cases

### 1. Cost Reduction
Cache frequently accessed API responses locally, reducing upstream API calls by 80-95%.

**Before:**
- 1M API requests/month
- $0.003 per request
- **Cost: $3,000/month**

**After (with apiproxyd):**
- 950K requests served from cache (free)
- 50K upstream requests
- **Cost: $150/month** (95% savings!)

### 2. Performance Improvement
Serve cached responses in <5ms instead of waiting 200ms+ for upstream APIs.

```
Cache Hit:  <5ms   ████
Cache Miss: 200ms  ████████████████████████████████████████
```

### 3. Offline Capability
Configure critical endpoints to work offline using cached data.

```bash
# Designate offline endpoints in config.json
"offline_endpoints": [
  "/v1/darkapi/ip/*",
  "/v1/geoip/*"
]

# Requests continue working even without internet
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8
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
apiproxy daemon start     # Start background service
apiproxy daemon stop      # Stop daemon
apiproxy daemon status    # Check daemon status
apiproxy daemon restart   # Restart daemon
```

### API Requests
```bash
apiproxy api GET /v1/darkapi/ip/8.8.8.8
apiproxy api POST /v1/nerdapi/hash --data '{"value":"test"}'
apiproxy api GET /v1/status --no-cache      # Bypass cache
apiproxy api GET /v1/ip/1.1.1.1 --cache-only # Only from cache
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
| `offline_endpoints` | array | Endpoints that work offline | `[]` |
| `whitelisted_endpoints` | array | Allowed endpoints | `[]` |

See [config.json.example](config.json.example) for complete example.

## Deployment

### Development
```bash
make build
./apiproxy daemon start
```

### Production (systemd)
```bash
# Install
sudo make install

# Create systemd service
sudo cp deploy/apiproxyd.service /etc/systemd/system/
sudo systemctl enable apiproxyd
sudo systemctl start apiproxyd
```

### Docker
```bash
# Build image
make docker-build

# Run container
docker run -p 9002:9002 \
  -v $(pwd)/config.json:/app/config.json:ro \
  apiproxyd:latest
```

See [DEPLOYMENT.md](DEPLOYMENT.md) for complete deployment guide.

## Performance Benchmarks

### Cache Performance (SQLite)
- **Cache Hit**: <5ms (local disk read)
- **Cache Miss**: ~200ms (upstream API call)
- **Throughput**: 10K-100K requests/sec (cached)
- **Storage**: ~1KB per cached response

### Cache Performance (PostgreSQL)
- **Cache Hit**: ~10ms (network + query)
- **Cache Miss**: ~200ms (upstream API call)
- **Throughput**: Scales horizontally
- **Storage**: Unlimited (database capacity)

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
  "version": "0.1.0",
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

- **Documentation**: [ARCHITECTURE.md](ARCHITECTURE.md), [DEPLOYMENT.md](DEPLOYMENT.md), [INSTALL.md](INSTALL.md)
- **Issues**: [GitHub Issues](https://github.com/afterdarktech/apiproxyd/issues)
- **Main Site**: [api.apiproxy.app](https://api.apiproxy.app)

## Roadmap

- [ ] Prometheus metrics exporter
- [ ] Grafana dashboard templates
- [ ] Kubernetes Helm charts
- [ ] Cache warming functionality
- [ ] Intelligent TTL adjustment
- [ ] Response compression
- [ ] Multi-tenancy support
- [ ] Web UI for management

## Related Projects

- [api.apiproxy.app](https://github.com/afterdarktech/apiproxy.app) - Main API gateway service
- [darkapi.io](https://darkapi.io) - IP intelligence API
- [nerdapi.io](https://nerdapi.io) - Developer utilities API

---

**Made with ❤️ by After Dark Systems, LLC**
