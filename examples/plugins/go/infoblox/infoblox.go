package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// InfobloxPlugin optimizes caching for Infoblox NIOS/WAPI API calls.
// Infoblox WAPI is a RESTful API for DNS, DHCP, and IPAM management.
//
// Caching Strategy:
// - Network/subnet queries: Cache for 1 hour (infrastructure changes rarely)
// - DNS record lookups: Cache for 5 minutes (DNS can change frequently)
// - DHCP lease queries: Cache for 2 minutes (leases are dynamic)
// - Grid/config queries: Cache for 30 minutes (config changes infrequently)
// - Object create/update/delete: No caching (POST/PUT/DELETE/PATCH)
//
// Reference: https://www.infoblox.com/wp-content/uploads/infoblox-deployment-infoblox-rest-api.pdf
type InfobloxPlugin struct {
	config map[string]interface{}
}

// Infoblox WAPI object types
var (
	// Network objects (cache longer - infrastructure is stable)
	networkObjectsRegex = regexp.MustCompile(`/wapi/v[0-9.]+/(network|networkcontainer|ipv6network)`)

	// DNS records (cache moderately - DNS changes regularly)
	dnsRecordsRegex = regexp.MustCompile(`/wapi/v[0-9.]+/(record:a|record:aaaa|record:ptr|record:cname|record:mx|record:txt|record:srv)`)

	// DHCP objects (cache briefly - leases are dynamic)
	dhcpObjectsRegex = regexp.MustCompile(`/wapi/v[0-9.]+/(lease|fixedaddress|range|dhcpserver)`)

	// Grid/config (cache moderately - config is relatively stable)
	gridObjectsRegex = regexp.MustCompile(`/wapi/v[0-9.]+/(grid|member|zone_auth|view)`)

	// Search operations (cache briefly - data may be time-sensitive)
	searchOperationsRegex = regexp.MustCompile(`/wapi/v[0-9.]+/search`)
)

func NewPlugin() plugin.Plugin {
	return &InfobloxPlugin{}
}

func (p *InfobloxPlugin) Name() string    { return "infoblox" }
func (p *InfobloxPlugin) Version() string { return "1.0.0" }

func (p *InfobloxPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[Infoblox Plugin] Initialized for NIOS/WAPI API caching\n")
	fmt.Printf("[Infoblox Plugin] Cache TTLs: Network=3600s, DNS=300s, DHCP=120s, Grid=1800s\n")
	return nil
}

func (p *InfobloxPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Only cache GET requests (read operations)
	if req.Method != "GET" {
		// Don't cache mutations
		if req.Metadata == nil {
			req.Metadata = make(map[string]string)
		}
		req.Metadata["skip_cache"] = "true"
		return req, true, nil
	}

	// Strip session cookies from cache key (identity-independent caching)
	// Infoblox uses cookies like 'ibapauth' for authentication
	if req.Headers != nil {
		delete(req.Headers, "Cookie")
		delete(req.Headers, "ibapauth")
	}

	return req, true, nil
}

func (p *InfobloxPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// Only process successful GET requests
	if req.Method != "GET" || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}

	// Determine cache TTL based on API endpoint
	var cacheTTL int

	switch {
	case networkObjectsRegex.MatchString(req.Endpoint):
		// Network objects: 1 hour (3600 seconds)
		// Networks and subnets change infrequently
		cacheTTL = 3600

	case dnsRecordsRegex.MatchString(req.Endpoint):
		// DNS records: 5 minutes (300 seconds)
		// DNS can be updated regularly
		cacheTTL = 300

	case dhcpObjectsRegex.MatchString(req.Endpoint):
		// DHCP leases: 2 minutes (120 seconds)
		// Leases are highly dynamic
		cacheTTL = 120

	case gridObjectsRegex.MatchString(req.Endpoint):
		// Grid/config: 30 minutes (1800 seconds)
		// Infrastructure config changes less frequently
		cacheTTL = 1800

	case searchOperationsRegex.MatchString(req.Endpoint):
		// Search operations: 1 minute (60 seconds)
		// Search results may be time-sensitive
		cacheTTL = 60

	default:
		// Default for other Infoblox API calls: 10 minutes
		cacheTTL = 600
	}

	// Set Cache-Control header
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Headers["Cache-Control"] = fmt.Sprintf("public, max-age=%d", cacheTTL)
	resp.Headers["X-Infoblox-Cache-TTL"] = fmt.Sprintf("%d", cacheTTL)

	// Add metadata for observability
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]string)
	}
	resp.Metadata["infoblox_cache_ttl"] = fmt.Sprintf("%d", cacheTTL)
	resp.Metadata["infoblox_endpoint_type"] = p.classifyEndpoint(req.Endpoint)

	return resp, nil
}

func (p *InfobloxPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// Add cache hit marker for Infoblox queries
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]string)
	}
	resp.Metadata["infoblox_cache_hit"] = "true"

	return resp, nil
}

func (p *InfobloxPlugin) Shutdown() error {
	fmt.Println("[Infoblox Plugin] Shutting down")
	return nil
}

// classifyEndpoint determines the type of Infoblox endpoint for metrics
func (p *InfobloxPlugin) classifyEndpoint(endpoint string) string {
	switch {
	case networkObjectsRegex.MatchString(endpoint):
		return "network"
	case dnsRecordsRegex.MatchString(endpoint):
		return "dns_record"
	case dhcpObjectsRegex.MatchString(endpoint):
		return "dhcp"
	case gridObjectsRegex.MatchString(endpoint):
		return "grid_config"
	case searchOperationsRegex.MatchString(endpoint):
		return "search"
	default:
		// Check if it's a WAPI endpoint at all
		if strings.Contains(endpoint, "/wapi/") {
			return "wapi_other"
		}
		return "unknown"
	}
}

func main() {
	// Required for Go plugins
}
