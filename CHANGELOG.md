# Changelog

All notable changes to apiproxyd will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Historical entries describe the intended feature set at the time. The current
support boundary and unverified subsystems are documented in
[README.md](README.md) and [BUGS.md](BUGS.md).

## [Unreleased]

### Added

- Optional SQLite-backed LLM context sessions, event storage, context packet
  building, and exact response caching.
- Regression tests for configuration merging and redaction, cache expiry and
  clearing, request behavior, and daemon startup.
- Multi-stage, non-root Docker packaging and reproducible Go toolchain metadata.
- A maintained known-limitations handoff in `BUGS.md`.

### Changed

- Updated the supported Go version and direct dependencies.
- Updated default `api.apiproxy.app` endpoint patterns.
- Documented the project as beta and separated tested core behavior from
  experimental plugins and distributed features.
- Made partial configuration files inherit safe defaults and expanded `~` paths.
- Limited automatic caching to safe `GET` and `HEAD` requests.
- Made CLI output script-friendly and removed decorative status emoji.

### Fixed

- Honored custom configuration paths and environment overrides consistently.
- Corrected cache TTL and stale-entry handling across memory and SQLite layers.
- Made cache clearing remove persisted entries.
- Included request bodies in deduplication identity.
- Prevented callers from overriding configured upstream credentials.
- Decompressed gzip responses correctly and improved startup/port-conflict
  error reporting.
- Removed the fake successful OAuth device-flow behavior; the unsupported mode
  now fails explicitly.

### Security

- Refuse non-loopback listeners unless `security.allow_remote` is explicitly
  enabled.
- Redact API keys and nested plugin secrets from configuration output.
- Updated dependencies to remove all vulnerabilities reachable from application
  code according to `govulncheck`.

## [0.3.0] - 2026-03-04

### Added
- **Infoblox Plugin**: Complete caching support for Infoblox NIOS/WAPI API
  - Intelligent TTL based on object type (networks: 1h, DNS: 5m, DHCP: 2m, grid: 30m)
  - Identity-independent caching with auth cookie stripping
  - Automatic mutation detection for POST/PUT/DELETE operations
  - Support for network, DNS, DHCP, grid, and search endpoints
- **BlueCat Plugin**: Complete caching support for BlueCat Address Manager DDIP API
  - Intelligent TTL based on endpoint type (config: 1h, DNS: 5m, DHCP: 2m, searches: 10m)
  - Identity-independent caching with Authorization header stripping
  - Mutation detection for add*/update*/delete*/deploy* operations
  - Support for configuration objects, DNS records, DHCP, searches, and deployment status
- **LDAP Authentication Middleware**: Enterprise directory integration
  - Full LDAP/Active Directory authentication support
  - Connection pooling with configurable pool size (default: 10)
  - TLS/SSL support (LDAPS port 636 and StartTLS port 389)
  - Group membership validation
  - Authentication result caching with configurable TTL
  - Automatic connection health checks and lifecycle management
  - HTTP Basic Auth integration
- **Plugin Documentation**: Comprehensive PLUGINS_README.md with:
  - Plugin system architecture documentation
  - All official plugin documentation (Infoblox, BlueCat, LDAP, Docker Registry, AWS SigV4, Kubernetes, OpenAI)
  - Plugin development guide with templates
  - Configuration examples and best practices
  - Troubleshooting guide and compatibility matrix
- **LDAP Configuration Example**: config.ldap.example.json with complete LDAP setup
- **Implementation Summary**: IMPLEMENTATION_SUMMARY_2026.md with full system documentation

### Changed
- **go.mod**: Added `github.com/go-ldap/ldap/v3 v3.4.6` dependency for LDAP support
- **README.md**: Updated to include new plugins and LDAP authentication

### Security
- LDAP authentication with TLS/SSL encryption
- Secure connection pooling with automatic health checks
- Authentication result caching to reduce LDAP server load

## [0.2.0] - 2026-01-14

### Added
- **gRPC Cluster Support**: Distributed caching with cluster.proto
  - HealthCheck service for node monitoring
  - InvalidateCache service for distributed cache invalidation
  - CacheLookup service for cross-node cache queries
- **Two-Tier Caching Architecture**:
  - L1 in-memory LRU cache (default 1000 entries, <1ms access)
  - L2 persistent cache (SQLite/PostgreSQL, <5ms access)
  - Automatic cache promotion from L2 to L1
- **Rate Limiting**: Token bucket algorithm with per-IP and per-API-key limits
- **OAuth2 Placeholder**: An incomplete device-flow mock was present; it was
  removed from the supported scope in the subsequent cleanup.
- **API Key Management**: Bcrypt hashing, multi-tier support (Free, Starter, Professional, Business, Enterprise)
- **SSL/TLS Support**:
  - TLS 1.2+ only
  - Modern cipher suites (ECDHE-RSA/ECDSA with AES-GCM)
  - HTTP/2 support
  - Security headers (X-Frame-Options, CSP, HSTS, etc.)
- **Security Features**:
  - SSRF protection with host allowlisting and private IP blocking
  - Request/response size limits (10MB request, 50MB response)
  - Circuit breaker for cascading failure prevention
  - Request deduplication
- **Cloud Vendor Plugins**:
  - AWS SigV4 proxy with identity-aware caching
  - Docker Registry with immutable blob caching
  - Kubernetes cache with materialized views
  - Cloudflare ETag conditional caching
  - Vercel API query normalization
  - Dartnode API token normalization
  - OpenAI adapter with cost tracking
- **Monitoring & Observability**:
  - Health check endpoint (/health)
  - Cache statistics endpoint (/cache/stats)
  - Prometheus metrics export (/metrics)
  - Audit logging for requests and responses
- **Background Job Processing**: Support for Asynq and River queue systems
- **Web Admin UI Plugin**: Real-time debugging interface (port 9003)

### Changed
- Improved cache performance with layered architecture
- Enhanced plugin system with Go and Python support
- Better error handling and recovery mechanisms

### Performance
- Historical release notes claimed throughput and latency figures without a
  checked-in reproducible benchmark. Those figures are not considered verified.

## [0.1.0] - Initial Release

### Added
- Basic API proxy functionality
- SQLite cache backend
- Simple configuration system
- CLI interface with daemon mode
- Health check endpoints

---

## Version Numbering

- **Major version (X.0.0)**: Breaking changes, major feature additions
- **Minor version (0.X.0)**: New features, backward-compatible
- **Patch version (0.0.X)**: Bug fixes, documentation updates

## Support

- **GitHub Repository**: https://github.com/afterdarksys/apiproxyd
- **Documentation**: https://apiproxy.app/docs
- **Support Email**: support@apiproxy.app
- **Sales**: sales@apiproxy.app

## License

This project is licensed under the MIT License - see the LICENSE file for details.
