# apiproxyd Deployment Guide

## Quick Start

### 1. Installation

```bash
# Clone or download the repository
cd /Users/ryan/development/apiproxyd

# Build the binary
go build -o apiproxy main.go

# Install to system path (optional)
sudo mv apiproxy /usr/local/bin/
```

### 2. Configuration

Create a `config.json` in your working directory or `~/.apiproxy/config.json`:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9002,
    "read_timeout": 15,
    "write_timeout": 15
  },
  "entry_point": "https://api.apiproxy.app",
  "api_key": "apx_live_your_api_key_here",
  "cache": {
    "backend": "sqlite",
    "path": "~/.apiproxy/cache.db",
    "ttl": 86400
  },
  "offline_endpoints": [
    "/v1/darkapi/ip/*",
    "/v1/nerdapi/hash",
    "/health",
    "/status"
  ],
  "whitelisted_endpoints": [
    "/v1/darkapi/*",
    "/v1/nerdapi/*",
    "/v1/computeapi/*"
  ]
}
```

Or use the example:
```bash
cp config.json.example config.json
# Edit config.json with your API key
```

### 3. Authentication

```bash
# Login with your API key
apiproxy login --api-key apx_live_xxxxx

# Or login interactively
apiproxy login
```

### 4. Start Daemon

```bash
# Start the daemon
apiproxy daemon start

# Check status
apiproxy daemon status

# View logs (if running in foreground)
apiproxy daemon start --foreground
```

### 5. Test

```bash
# Run diagnostics
apiproxy test

# Make a test request
apiproxy api GET /v1/darkapi/ip/8.8.8.8

# Or via the daemon
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxxxx"
```

## Configuration Reference

### config.json Structure

```json
{
  "server": {
    "host": "127.0.0.1",        // Listen address (use 0.0.0.0 for all interfaces)
    "port": 9002,                // Listen port
    "read_timeout": 15,          // Request read timeout (seconds)
    "write_timeout": 15          // Response write timeout (seconds)
  },
  "entry_point": "https://api.apiproxy.app",  // Upstream API endpoint
  "api_key": "apx_live_xxx",                   // Your API key from api.apiproxy.app
  "cache": {
    "backend": "sqlite",                        // "sqlite" or "postgres"
    "path": "~/.apiproxy/cache.db",            // SQLite database path
    "ttl": 86400,                               // Cache TTL in seconds (24h default)
    "postgres_dsn": "host=localhost..."         // PostgreSQL connection string (if using postgres)
  },
  "offline_endpoints": [
    "/v1/darkapi/ip/*",          // Endpoints that work offline from cache
    "/health"                     // Wildcards supported with *
  ],
  "whitelisted_endpoints": [
    "/v1/darkapi/*",              // Endpoints allowed to be proxied
    "/v1/nerdapi/*"               // Requests to non-whitelisted endpoints return 403
  ]
}
```

### Cache Backends

#### SQLite (Default)
```json
"cache": {
  "backend": "sqlite",
  "path": "~/.apiproxy/cache.db",
  "ttl": 86400
}
```

**Pros:**
- Zero configuration
- No external dependencies
- Perfect for single-server deployments
- Fast (10K-100K ops/sec)

**Cons:**
- Not shared across multiple servers
- Limited to single host

#### PostgreSQL (Enterprise)
```json
"cache": {
  "backend": "postgres",
  "postgres_dsn": "host=localhost port=5432 user=apiproxy password=xxx dbname=apiproxy_cache sslmode=disable",
  "ttl": 86400
}
```

**Pros:**
- Shared cache across multiple servers
- Scalable and reliable
- JSONB metadata support
- Can use existing PostgreSQL infrastructure

**Cons:**
- Requires PostgreSQL server
- More complex setup

**PostgreSQL Setup:**
```sql
-- Connect to PostgreSQL
psql -U postgres

-- Create database and user
CREATE DATABASE apiproxy_cache;
CREATE USER apiproxy WITH PASSWORD 'secure_password_here';
GRANT ALL PRIVILEGES ON DATABASE apiproxy_cache TO apiproxy;

-- The schema is created automatically by apiproxyd
```

## Deployment Scenarios

### Scenario 1: Development Machine

**Use Case:** Local API testing and development

```bash
# Use default SQLite cache
apiproxy daemon start

# Make requests via CLI
apiproxy api GET /v1/darkapi/ip/8.8.8.8
```

**Benefits:**
- Fast setup
- No external dependencies
- Persistent cache across sessions

---

### Scenario 2: Single Production Server

**Use Case:** Small business with one application server

**config.json:**
```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 9002
  },
  "entry_point": "https://api.apiproxy.app",
  "api_key": "apx_live_prod_xxx",
  "cache": {
    "backend": "sqlite",
    "path": "/var/lib/apiproxy/cache.db",
    "ttl": 86400
  }
}
```

**Systemd Service:** `/etc/systemd/system/apiproxyd.service`
```ini
[Unit]
Description=API Proxy Cache Daemon
After=network.target

[Service]
Type=simple
User=apiproxy
Group=apiproxy
WorkingDirectory=/etc/apiproxy
ExecStart=/usr/local/bin/apiproxy daemon start
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

**Setup:**
```bash
# Create user
sudo useradd -r -s /bin/false apiproxy

# Create directories
sudo mkdir -p /etc/apiproxy /var/lib/apiproxy
sudo chown apiproxy:apiproxy /var/lib/apiproxy

# Copy config
sudo cp config.json /etc/apiproxy/
sudo chown apiproxy:apiproxy /etc/apiproxy/config.json

# Install binary
sudo cp apiproxy /usr/local/bin/
sudo chmod +x /usr/local/bin/apiproxy

# Enable and start service
sudo systemctl enable apiproxyd
sudo systemctl start apiproxyd
sudo systemctl status apiproxyd
```

---

### Scenario 3: Multi-Server Production (PostgreSQL)

**Use Case:** Load-balanced application with 3+ servers

**Architecture:**
```
┌─────────────┐
│ Load Balancer│
└──────┬──────┘
       │
   ┌───┴───┬───────┬────────┐
   │       │       │        │
┌──▼──┐ ┌──▼──┐ ┌──▼──┐  ┌──▼──────────┐
│App1 │ │App2 │ │App3 │  │ PostgreSQL  │
│+APD │ │+APD │ │+APD │  │ (Shared     │
└─────┘ └─────┘ └─────┘  │  Cache)     │
                          └─────────────┘
APD = apiproxyd
```

**config.json (on each server):**
```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 9002
  },
  "entry_point": "https://api.apiproxy.app",
  "api_key": "apx_live_prod_xxx",
  "cache": {
    "backend": "postgres",
    "postgres_dsn": "host=postgres.internal.company.com port=5432 user=apiproxy password=xxx dbname=apiproxy_cache sslmode=require",
    "ttl": 86400
  }
}
```

**Benefits:**
- Shared cache across all servers
- Request on Server1 caches for Server2 and Server3
- Consistent cache state
- Higher cache hit rate

---

### Scenario 4: Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY . .
RUN go build -o apiproxy main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates
COPY --from=builder /build/apiproxy /usr/local/bin/

EXPOSE 9002
ENTRYPOINT ["apiproxy", "daemon", "start"]
```

**docker-compose.yml:**
```yaml
version: '3.8'

services:
  apiproxyd:
    build: .
    ports:
      - "9002:9002"
    volumes:
      - ./config.json:/app/config.json:ro
      - cache-data:/var/lib/apiproxy
    environment:
      - APIPROXY_API_KEY=${API_KEY}
    restart: unless-stopped

volumes:
  cache-data:
```

**Usage:**
```bash
# Set API key
echo "API_KEY=apx_live_xxx" > .env

# Start service
docker-compose up -d

# View logs
docker-compose logs -f apiproxyd

# Make requests
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxx"
```

---

## Advanced Features

### Offline Endpoints

Endpoints in the `offline_endpoints` list work without internet connectivity:

```json
"offline_endpoints": [
  "/v1/darkapi/ip/*",
  "/health"
]
```

**Behavior:**
1. Requests to offline endpoints are **only served from cache**
2. If not in cache, returns `503 Service Unavailable`
3. Use this for mission-critical data that must be pre-cached
4. Ideal for disaster recovery scenarios

**Example:**
```bash
# Pre-populate cache
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8

# Disconnect from internet
# Request still works from cache
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8
# Response includes: X-Offline: true
```

### Whitelisted Endpoints

Only whitelisted endpoints can be proxied:

```json
"whitelisted_endpoints": [
  "/v1/darkapi/*",
  "/v1/nerdapi/*"
]
```

**Benefits:**
- Security: Prevents unauthorized API access
- Cost control: Limits which APIs can be called
- Compliance: Enforce API usage policies

**Example:**
```bash
# Whitelisted - works
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8

# Not whitelisted - returns 403
curl http://localhost:9002/api/v1/randomapi/endpoint
# Error: Endpoint not whitelisted
```

---

## Monitoring

### Health Check

```bash
curl http://localhost:9002/health
```

**Response:**
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

**Response:**
```json
{
  "entries": 1234,
  "size_bytes": 567890,
  "hit_rate": 0.85,
  "hits": 10000,
  "misses": 1500
}
```

### Response Headers

Every proxied response includes:
- `X-Cache: HIT` - Response served from cache
- `X-Cache: MISS` - Response fetched from upstream
- `X-Offline: true` - Offline endpoint served from cache

---

## Troubleshooting

### Daemon Won't Start

**Check logs:**
```bash
apiproxy daemon start --foreground
```

**Common issues:**
- Port already in use: `lsof -i :9002`
- Config file not found: Check `config.json` exists
- Invalid config: Run `apiproxy test`

### Cache Not Working

```bash
# Check cache stats
curl http://localhost:9002/cache/stats

# Run diagnostics
apiproxy test

# Clear cache and retry
curl -X POST http://localhost:9002/cache/clear
```

### Authentication Failures

```bash
# Validate API key
apiproxy login --api-key apx_live_xxx

# Check config
apiproxy config show

# Test directly
curl https://api.apiproxy.app/v1/validate \
  -H "X-API-Key: apx_live_xxx"
```

---

## Security Best Practices

1. **API Key Protection**
   - Store in config file with `chmod 600`
   - Never commit to version control
   - Use environment variables in production

2. **Network Security**
   - Bind to `127.0.0.1` for local-only access
   - Use firewall rules for external access
   - Consider TLS termination proxy (nginx, Caddy)

3. **Cache Security**
   - Restrict file permissions: `chmod 600 cache.db`
   - Use PostgreSQL with strong passwords
   - Enable SSL for PostgreSQL connections

4. **Update Regularly**
   ```bash
   cd /path/to/apiproxyd
   git pull
   go build -o apiproxy main.go
   sudo systemctl restart apiproxyd
   ```

---

## Performance Tuning

### SQLite Optimization

```json
"cache": {
  "backend": "sqlite",
  "path": "/mnt/fast-ssd/apiproxy.db",
  "ttl": 86400
}
```

- Use SSD storage for cache database
- Increase OS file descriptor limits
- Monitor disk I/O with `iostat`

### PostgreSQL Optimization

```sql
-- Increase connection pool
ALTER SYSTEM SET max_connections = 100;

-- Enable query caching
ALTER SYSTEM SET shared_buffers = '256MB';

-- Create indexes
CREATE INDEX idx_apiproxy_cache_path ON apiproxy_cache(path);
CREATE INDEX idx_apiproxy_cache_expires ON apiproxy_cache(expires_at);
```

### HTTP Tuning

```json
"server": {
  "read_timeout": 30,
  "write_timeout": 30
}
```

- Increase timeouts for slow APIs
- Monitor request latency
- Use keep-alive connections

---

## Next Steps

- Set up monitoring (Prometheus, Grafana)
- Implement log aggregation (ELK, Loki)
- Configure backup for cache database
- Test failover scenarios
- Plan capacity for growth
