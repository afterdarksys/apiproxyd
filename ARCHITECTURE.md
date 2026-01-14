# apiproxyd Architecture

## Overview

`apiproxyd` is an on-premises API caching daemon that acts as a companion to **api.apiproxy.app**. It provides businesses with local caching capabilities to reduce API costs, improve performance, and maintain high availability.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     Business Network                         │
│                                                              │
│  ┌──────────────┐                                           │
│  │ Application  │                                           │
│  └──────┬───────┘                                           │
│         │                                                    │
│         ▼                                                    │
│  ┌──────────────────────────────────────────┐              │
│  │         apiproxyd (Port 9002)            │              │
│  │  ┌────────────┐  ┌──────────────────┐   │              │
│  │  │ HTTP Proxy │  │  Cache Manager   │   │              │
│  │  └─────┬──────┘  └────────┬─────────┘   │              │
│  │        │                  │              │              │
│  │        │         ┌────────▼─────────┐   │              │
│  │        │         │  SQLite/Postgres │   │              │
│  │        │         │   Cache Storage  │   │              │
│  │        │         └──────────────────┘   │              │
│  └────────┼──────────────────────────────┘              │
│           │                                                │
└───────────┼────────────────────────────────────────────────┘
            │
            │ HTTPS
            ▼
┌───────────────────────────────────┐
│    api.apiproxy.app (Cloud)       │
│                                   │
│  ┌─────────────────────────────┐ │
│  │  KrakenD API Gateway        │ │
│  └────────────┬────────────────┘ │
│               │                   │
│  ┌────────────▼────────────────┐ │
│  │  Auth Service               │ │
│  └────────────┬────────────────┘ │
│               │                   │
│  ┌────────────▼────────────────┐ │
│  │  Backend APIs               │ │
│  │  (darkapi, nerdapi, etc.)   │ │
│  └─────────────────────────────┘ │
└───────────────────────────────────┘
```

## Components

### 1. CLI Interface (`cmd/`)

The command-line interface provides user interaction:

- **login** - Authenticate with api.apiproxy.app
- **api** - Make API requests (with caching)
- **daemon** - Control background service
- **console** - Interactive REPL
- **config** - Manage configuration
- **test** - Run diagnostics

### 2. Cache Layer (`pkg/cache/`)

Dual-backend caching system:

#### SQLite Backend
- **Use case**: Single-server deployments, development
- **Storage**: Local file (`~/.apiproxy/cache.db`)
- **Performance**: ~100K ops/sec
- **Features**: Zero configuration, embedded

#### PostgreSQL Backend
- **Use case**: Multi-server deployments, enterprise
- **Storage**: Shared PostgreSQL database
- **Performance**: Scalable, connection pooled
- **Features**: JSONB metadata, partitioning support

#### Cache Schema
```sql
CREATE TABLE cache_entries (
    key TEXT PRIMARY KEY,              -- SHA256 hash of (method + path + body)
    value BLOB NOT NULL,               -- Cached API response
    method TEXT NOT NULL,              -- HTTP method
    path TEXT NOT NULL,                -- API endpoint path
    request_body TEXT,                 -- Original request body
    status_code INTEGER,               -- HTTP status code
    created_at TIMESTAMP,              -- When cached
    expires_at TIMESTAMP NOT NULL,     -- Expiration time
    metadata JSONB                     -- Additional metadata
);
```

### 3. HTTP Client (`pkg/client/`)

API communication layer:

- **Authentication**: X-API-Key header
- **Endpoints**:
  - `/v1/validate` - API key validation
  - `/v1/*` - Proxied API requests
- **Features**:
  - Automatic retry with backoff
  - Request/response logging
  - Error handling

### 4. Daemon Service (`pkg/daemon/`)

Background HTTP proxy server:

#### Endpoints

**Health Check**
```
GET /health
Response: {"status": "ok", "version": "0.1.0"}
```

**API Proxy**
```
GET /api/v1/darkapi/ip/8.8.8.8
Headers: X-API-Key: apx_live_xxx
Response: (cached or fresh API response)
Headers: X-Cache: HIT|MISS
```

**Cache Statistics**
```
GET /cache/stats
Response: {
  "entries": 1234,
  "size_bytes": 567890,
  "hit_rate": 0.85
}
```

**Cache Clear**
```
POST /cache/clear
Response: {"status": "cleared"}
```

### 5. Configuration (`pkg/config/`)

YAML-based configuration stored in `~/.apiproxy/config.yml`:

```yaml
# API Configuration
api_key: apx_live_xxxxx
endpoint: https://api.apiproxy.app

# Cache Configuration
cache_backend: sqlite  # or "postgres"
cache_path: /home/user/.apiproxy/cache.db
cache_ttl: 86400       # 24 hours in seconds

# Daemon Configuration
daemon_host: 127.0.0.1
daemon_port: 9002

# PostgreSQL (if using postgres backend)
postgres_dsn: "host=localhost port=5432 user=apiproxy dbname=apiproxy_cache"
```

## Request Flow

### Cache Hit
```
1. Application → apiproxyd (GET /api/v1/darkapi/ip/8.8.8.8)
2. apiproxyd generates cache key: sha256(GET + path + body)
3. Cache lookup → Found (not expired)
4. Return cached response with X-Cache: HIT
5. Total time: <5ms
```

### Cache Miss
```
1. Application → apiproxyd (GET /api/v1/darkapi/ip/8.8.8.8)
2. apiproxyd generates cache key: sha256(GET + path + body)
3. Cache lookup → Not found
4. apiproxyd → api.apiproxy.app (with X-API-Key)
5. api.apiproxy.app validates key, makes request to darkapi.io
6. Response returned to apiproxyd
7. apiproxyd stores response in cache (TTL: 24 hours)
8. Return response with X-Cache: MISS
9. Total time: ~200ms
```

## Deployment Modes

### Mode 1: On-Demand CLI
```bash
# Make requests directly via CLI
apiproxy api GET /v1/darkapi/ip/8.8.8.8

# Cache is checked automatically
# No daemon required
```

### Mode 2: Daemon Service
```bash
# Start daemon in background
apiproxy daemon start

# Applications make requests to local proxy
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxx"

# Daemon handles caching automatically
```

### Mode 3: System Service (Production)
```bash
# Install as systemd service
sudo systemctl enable apiproxyd
sudo systemctl start apiproxyd

# Runs automatically on boot
# Survives reboots and crashes
```

## Cache Strategy

### Key Generation
```go
key = sha256(method + path + request_body)
```

- **Identical requests** → Same key → Cache hit
- **Different parameters** → Different key → Cache miss
- **Body order matters** → Normalize JSON before hashing

### TTL (Time To Live)
- **Default**: 24 hours
- **Configurable**: Via `cache_ttl` config
- **Per-endpoint override**: Coming soon

### Eviction
- **Automatic**: Expired entries removed on read
- **Manual**: `apiproxy cache clear` or `POST /cache/clear`
- **Scheduled**: Cleanup job every hour (daemon mode)

## Security

### API Key Storage
- Stored in `~/.apiproxy/config.yml`
- File permissions: `0600` (owner read/write only)
- Never logged or exposed in responses

### Network Security
- **Local daemon**: Binds to `127.0.0.1` by default
- **TLS**: All upstream requests use HTTPS
- **Headers**: API key sent via `X-API-Key` header

### Cache Poisoning Prevention
- Cache keys include full request context
- No user-controlled cache keys
- TTL prevents stale data

## Performance

### Benchmarks (SQLite)
- **Cache hit**: <5ms (local disk read)
- **Cache miss**: ~200ms (upstream API call)
- **Cache write**: <10ms (background)
- **Throughput**: ~10K requests/sec (cached)

### Optimization
- **Connection pooling**: Reuse HTTP connections
- **Parallel requests**: Non-blocking I/O
- **Index usage**: Fast lookups on `key`, `expires_at`

## Monitoring

### Metrics (Coming Soon)
- Request rate (requests/sec)
- Cache hit rate (%)
- Average latency (ms)
- Storage usage (bytes)

### Logging
- **Debug mode**: `apiproxy --debug`
- **Daemon logs**: Stdout/stderr
- **Syslog**: When running as system service

## Scalability

### Single Server
- SQLite backend
- 10K-100K cached requests/sec
- Suitable for most deployments

### Multi-Server
- PostgreSQL backend
- Shared cache across servers
- Horizontal scaling
- Load balancer friendly

## Future Enhancements

### Planned Features
1. **Metrics dashboard** - Prometheus/Grafana integration
2. **Cache warming** - Pre-populate frequently accessed endpoints
3. **Intelligent TTL** - Adjust based on endpoint volatility
4. **Compression** - Compress cached responses
5. **Webhooks** - Invalidate cache on upstream changes
6. **Multi-tenancy** - Multiple API keys/accounts

### Integration Options
- **Docker** - Containerized deployment
- **Kubernetes** - StatefulSet with shared PostgreSQL
- **Terraform** - Infrastructure as code
- **Ansible** - Configuration management
