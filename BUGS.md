# Known Bugs and Limitations

Last reviewed: 2026-07-24

This is the handoff list for the next development pass. The local SQLite-backed
proxy is usable, but the repository still contains experimental subsystems that
should not be treated as production-ready.

## OpenAI Compatibility

apiproxyd is not currently OpenAI API compatible.

- It does not implement the standard OpenAI `/v1` API contract.
- It buffers responses and does not support Server-Sent Events streaming.
- The proxy does not faithfully preserve every upstream status code or header.
- Configured upstream authentication takes precedence over caller credentials.
- The example Python OpenAI adapter uses an incomplete plugin protocol and is
  not a supported compatibility layer.
- The `/llm` endpoints store local coding-agent context and exact cached
  responses; they do not provide model inference.

Supporting OpenAI clients would require a deliberate compatibility layer,
contract tests, streaming, error/status fidelity, and a clear credential model.

## Proxy and Cache Semantics

- Upstream failures are currently flattened into proxy errors instead of
  preserving the upstream response contract.
- Cache keys include method, endpoint, and request body, but not all
  representation-changing request headers. Endpoints that vary by headers can
  collide unless a plugin supplies an appropriate cache key.
- Only `GET` and `HEAD` are cached. This is intentional for safety, but older
  documentation and plugin examples may still imply mutation caching.
- Offline mode can only return entries already present and still available in
  the configured cache.

## Network Exposure

- Remote listening now requires `security.allow_remote: true`, but the HTTP
  routes are not a complete multi-user authorization boundary.
- If remote access is enabled, deploy behind an authenticating reverse proxy or
  another trusted network boundary.
- TLS configuration and optional LDAP support have not been exercised
  end-to-end in this cleanup pass.

## Experimental Subsystems

The following compile but were not validated against live services during this
cleanup:

- PostgreSQL shared cache
- clustering and distributed invalidation
- Asynq and River queues
- LDAP/Active Directory authentication
- third-party Go and Python plugins
- Infoblox, BlueCat, cloud-provider, and web-admin examples

Cluster peer broadcasting also contains placeholder behavior and needs
integration tests before it can be considered functional.

## Build and Security Notes

- No reproducible performance benchmark suite is checked in. Historical
  throughput and latency numbers should not be relied on.
- A prebuilt `apiproxy` binary is tracked in the repository and can lag behind
  source. Build from source or use the Dockerfile for a known-current binary.
- `govulncheck` reports no reachable vulnerability in the application after the
  dependency update. It still reports the module-level advisory `GO-2026-5932`
  for the unmaintained `golang.org/x/crypto/openpgp` package; that package is
  not imported by apiproxyd.

## Next Pickup

1. Decide whether OpenAI compatibility is a product goal or remove the adapter.
2. Preserve upstream status codes, relevant headers, and streaming bodies.
3. Add an explicit authentication middleware for remotely exposed routes.
4. Add live-service integration tests for every advertised optional subsystem.
5. Replace or remove placeholder cluster behavior.
6. Add reproducible benchmarks before publishing performance claims.
7. Stop tracking release binaries, or automate rebuilding them for releases.
