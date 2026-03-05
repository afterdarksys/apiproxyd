package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

// BlueCatPlugin optimizes caching for BlueCat Address Manager (BAM) and DNS/DHCP Server (BDDS) REST API.
// BlueCat provides DNS, DHCP, and IPAM (DDI) solutions through their Address Manager platform.
//
// Caching Strategy:
// - Configuration objects (IP blocks, networks): Cache for 1 hour (infrastructure is stable)
// - DNS zones and resource records: Cache for 5 minutes (DNS changes regularly)
// - DHCP ranges and reservations: Cache for 2 minutes (DHCP is dynamic)
// - Entity searches: Cache for 10 minutes (search results are generally stable)
// - Deployment status: Cache for 30 seconds (deployment state changes frequently)
// - Create/Update/Delete operations: No caching (POST/PUT/DELETE/PATCH)
//
// Reference: BlueCat Address Manager API Guide
// https://docs.bluecatnetworks.com/r/Address-Manager-API-Guide
type BlueCatPlugin struct {
	config map[string]interface{}
}

// BlueCat API endpoint patterns
var (
	// Configuration objects (networks, IP blocks, etc.)
	configObjectsRegex = regexp.MustCompile(`/Services/REST/v1/(getEntities|getEntityById|getEntityByName|getIPRangedByIP|getIP4Block|getIP6Block|getIP4Network|getIP6Network)`)

	// DNS zones and records
	dnsObjectsRegex = regexp.MustCompile(`/Services/REST/v1/(getZone|getHostRecord|getAliasRecord|getExternalHostRecord|getMXRecord|getTXTRecord|getSRVRecord|getResourceRecord)`)

	// DHCP ranges and reservations
	dhcpObjectsRegex = regexp.MustCompile(`/Services/REST/v1/(getDHCPRange|getDHCPReservedIPAddress|getDHCPClientDeploymentOption)`)

	// Entity searches (generic searches)
	searchOperationsRegex = regexp.MustCompile(`/Services/REST/v1/(searchByCategory|searchByObjectTypes)`)

	// Deployment operations (status checks, etc.)
	deploymentRegex = regexp.MustCompile(`/Services/REST/v1/(getDeploymentTask|getServerDeploymentStatus)`)

	// System and session (very stable)
	systemRegex = regexp.MustCompile(`/Services/REST/v1/(getSystemInfo|getVersion)`)
)

func NewPlugin() plugin.Plugin {
	return &BlueCatPlugin{}
}

func (p *BlueCatPlugin) Name() string    { return "bluecat" }
func (p *BlueCatPlugin) Version() string { return "1.0.0" }

func (p *BlueCatPlugin) Init(config map[string]interface{}) error {
	p.config = config
	fmt.Printf("[BlueCat Plugin] Initialized for Address Manager API caching\n")
	fmt.Printf("[BlueCat Plugin] Cache TTLs: Config=3600s, DNS=300s, DHCP=120s, Search=600s, Deploy=30s\n")
	return nil
}

func (p *BlueCatPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// BlueCat API uses both GET and POST for read operations
	// However, mutations (add*/update*/delete*) should never be cached
	isMutation := strings.HasPrefix(req.Endpoint, "/Services/REST/v1/add") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/update") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/delete") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/deploy")

	if isMutation {
		// Don't cache mutations
		if req.Metadata == nil {
			req.Metadata = make(map[string]string)
		}
		req.Metadata["skip_cache"] = "true"
		return req, true, nil
	}

	// Strip authentication token from cache key (identity-independent caching)
	// BlueCat uses Authorization header with BAMAuthToken
	if req.Headers != nil {
		delete(req.Headers, "Authorization")
		delete(req.Headers, "Cookie")
	}

	return req, true, nil
}

func (p *BlueCatPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// Only process successful responses for GET requests
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}

	// Check if this is a mutation operation
	isMutation := strings.HasPrefix(req.Endpoint, "/Services/REST/v1/add") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/update") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/delete") ||
		strings.HasPrefix(req.Endpoint, "/Services/REST/v1/deploy")

	if isMutation {
		// Don't cache mutations
		return resp, nil
	}

	// Determine cache TTL based on API endpoint
	var cacheTTL int

	switch {
	case configObjectsRegex.MatchString(req.Endpoint):
		// Configuration objects: 1 hour (3600 seconds)
		// IP blocks and networks are infrastructure and change infrequently
		cacheTTL = 3600

	case dnsObjectsRegex.MatchString(req.Endpoint):
		// DNS records: 5 minutes (300 seconds)
		// DNS records can be updated regularly
		cacheTTL = 300

	case dhcpObjectsRegex.MatchString(req.Endpoint):
		// DHCP objects: 2 minutes (120 seconds)
		// DHCP ranges and reservations are dynamic
		cacheTTL = 120

	case searchOperationsRegex.MatchString(req.Endpoint):
		// Search operations: 10 minutes (600 seconds)
		// Search results are generally stable
		cacheTTL = 600

	case deploymentRegex.MatchString(req.Endpoint):
		// Deployment status: 30 seconds
		// Deployment state changes frequently during deployments
		cacheTTL = 30

	case systemRegex.MatchString(req.Endpoint):
		// System info: 1 hour (3600 seconds)
		// System information is very stable
		cacheTTL = 3600

	default:
		// Default for other BlueCat API calls: 5 minutes
		cacheTTL = 300
	}

	// Set Cache-Control header
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Headers["Cache-Control"] = fmt.Sprintf("public, max-age=%d", cacheTTL)
	resp.Headers["X-BlueCat-Cache-TTL"] = fmt.Sprintf("%d", cacheTTL)

	// Add metadata for observability
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]string)
	}
	resp.Metadata["bluecat_cache_ttl"] = fmt.Sprintf("%d", cacheTTL)
	resp.Metadata["bluecat_endpoint_type"] = p.classifyEndpoint(req.Endpoint)

	return resp, nil
}

func (p *BlueCatPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	// Add cache hit marker for BlueCat queries
	if resp.Metadata == nil {
		resp.Metadata = make(map[string]string)
	}
	resp.Metadata["bluecat_cache_hit"] = "true"

	return resp, nil
}

func (p *BlueCatPlugin) Shutdown() error {
	fmt.Println("[BlueCat Plugin] Shutting down")
	return nil
}

// classifyEndpoint determines the type of BlueCat endpoint for metrics
func (p *BlueCatPlugin) classifyEndpoint(endpoint string) string {
	switch {
	case configObjectsRegex.MatchString(endpoint):
		return "config_object"
	case dnsObjectsRegex.MatchString(endpoint):
		return "dns_record"
	case dhcpObjectsRegex.MatchString(endpoint):
		return "dhcp_object"
	case searchOperationsRegex.MatchString(endpoint):
		return "search"
	case deploymentRegex.MatchString(endpoint):
		return "deployment"
	case systemRegex.MatchString(endpoint):
		return "system_info"
	default:
		// Check if it's a BlueCat API endpoint
		if strings.Contains(endpoint, "/Services/REST/") {
			return "rest_other"
		}
		return "unknown"
	}
}

func main() {
	// Required for Go plugins
}
