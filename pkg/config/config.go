package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Host         string `yaml:"host" json:"host"`
	Port         int    `yaml:"port" json:"port"`
	ReadTimeout  int    `yaml:"read_timeout" json:"read_timeout"`   // seconds
	WriteTimeout int    `yaml:"write_timeout" json:"write_timeout"` // seconds
	IdleTimeout  int    `yaml:"idle_timeout" json:"idle_timeout"`   // seconds
	// TLS configuration
	TLSEnabled  bool   `yaml:"tls_enabled" json:"tls_enabled"`
	TLSCertFile string `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file" json:"tls_key_file"`
	// HTTP/2 support (enabled by default with TLS)
	EnableHTTP2 bool `yaml:"enable_http2" json:"enable_http2"`
}

type CacheConfig struct {
	Backend     string `yaml:"backend" json:"backend"`
	Path        string `yaml:"path" json:"path"`
	TTL         int    `yaml:"ttl" json:"ttl"` // seconds
	PostgresDSN string `yaml:"postgres_dsn,omitempty" json:"postgres_dsn,omitempty"`
	// In-memory LRU cache configuration
	MemoryCacheEnabled bool `yaml:"memory_cache_enabled" json:"memory_cache_enabled"`
	MemoryCacheSize    int  `yaml:"memory_cache_size" json:"memory_cache_size"` // number of entries
	// Database connection pooling
	MaxOpenConns    int `yaml:"max_open_conns" json:"max_open_conns"`       // max open connections
	MaxIdleConns    int `yaml:"max_idle_conns" json:"max_idle_conns"`       // max idle connections
	ConnMaxLifetime int `yaml:"conn_max_lifetime" json:"conn_max_lifetime"` // seconds
	ConnMaxIdleTime int `yaml:"conn_max_idle_time" json:"conn_max_idle_time"` // seconds
	// Background cleanup
	CleanupInterval int `yaml:"cleanup_interval" json:"cleanup_interval"` // seconds
}

type PluginConfig struct {
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Plugins []PluginEntry  `yaml:"plugins" json:"plugins"`
}

type PluginEntry struct {
	Name    string                 `yaml:"name" json:"name"`
	Type    string                 `yaml:"type" json:"type"` // "go" or "python"
	Path    string                 `yaml:"path" json:"path"`
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Config  map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// SecurityConfig holds security-related settings
type SecurityConfig struct {
	// Rate limiting
	RateLimitEnabled     bool `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	RateLimitPerIP       int  `yaml:"rate_limit_per_ip" json:"rate_limit_per_ip"`           // requests per minute
	RateLimitPerKey      int  `yaml:"rate_limit_per_key" json:"rate_limit_per_key"`         // requests per minute
	RateLimitBurst       int  `yaml:"rate_limit_burst" json:"rate_limit_burst"`             // burst size
	// Request/response size limits
	MaxRequestBodySize  int64 `yaml:"max_request_body_size" json:"max_request_body_size"`   // bytes
	MaxResponseBodySize int64 `yaml:"max_response_body_size" json:"max_response_body_size"` // bytes
	// SSRF protection
	SSRFProtectionEnabled bool     `yaml:"ssrf_protection_enabled" json:"ssrf_protection_enabled"`
	AllowedUpstreamHosts  []string `yaml:"allowed_upstream_hosts" json:"allowed_upstream_hosts"`
	BlockPrivateIPs       bool     `yaml:"block_private_ips" json:"block_private_ips"`
	// Metrics authentication
	MetricsAuthEnabled bool   `yaml:"metrics_auth_enabled" json:"metrics_auth_enabled"`
	MetricsAuthToken   string `yaml:"metrics_auth_token" json:"metrics_auth_token"`
}

// ClientConfig holds HTTP client configuration
type ClientConfig struct {
	// Timeouts
	RequestTimeout  int `yaml:"request_timeout" json:"request_timeout"`   // seconds
	DialTimeout     int `yaml:"dial_timeout" json:"dial_timeout"`         // seconds
	KeepAlive       int `yaml:"keep_alive" json:"keep_alive"`             // seconds
	// Connection pooling
	MaxIdleConns        int `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int `yaml:"max_conns_per_host" json:"max_conns_per_host"`
	IdleConnTimeout     int `yaml:"idle_conn_timeout" json:"idle_conn_timeout"` // seconds
	// Circuit breaker
	CircuitBreakerEnabled    bool `yaml:"circuit_breaker_enabled" json:"circuit_breaker_enabled"`
	CircuitBreakerThreshold  int  `yaml:"circuit_breaker_threshold" json:"circuit_breaker_threshold"`   // consecutive failures
	CircuitBreakerTimeout    int  `yaml:"circuit_breaker_timeout" json:"circuit_breaker_timeout"`       // seconds
	CircuitBreakerHalfOpen   int  `yaml:"circuit_breaker_half_open" json:"circuit_breaker_half_open"`   // max requests in half-open
	// Request deduplication
	DeduplicationEnabled bool `yaml:"deduplication_enabled" json:"deduplication_enabled"`
}

type Config struct {
	// Server configuration
	Server ServerConfig `yaml:"server" json:"server"`

	// API configuration
	EntryPoint string `yaml:"entry_point" json:"entry_point"` // upstream API endpoint
	APIKey     string `yaml:"api_key" json:"api_key"`

	// Cache configuration
	Cache CacheConfig `yaml:"cache" json:"cache"`

	// Plugin configuration
	Plugins PluginConfig `yaml:"plugins,omitempty" json:"plugins,omitempty"`

	// Security configuration
	Security SecurityConfig `yaml:"security,omitempty" json:"security,omitempty"`

	// Client configuration
	Client ClientConfig `yaml:"client,omitempty" json:"client,omitempty"`

	// Offline endpoints - cached indefinitely, work without internet
	OfflineEndpoints []string `yaml:"offline_endpoints" json:"offline_endpoints"`

	// Whitelisted endpoints - allowed to be proxied
	WhitelistedEndpoints []string `yaml:"whitelisted_endpoints" json:"whitelisted_endpoints"`

	// Legacy fields for backward compatibility
	UserID string `yaml:"user_id,omitempty" json:"user_id,omitempty"`
	Tier   string `yaml:"tier,omitempty" json:"tier,omitempty"`

	// Deprecated fields (mapped to new structure)
	Endpoint     string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	CacheBackend string `yaml:"cache_backend,omitempty" json:"cache_backend,omitempty"`
	CachePath    string `yaml:"cache_path,omitempty" json:"cache_path,omitempty"`
	CacheTTL     int    `yaml:"cache_ttl,omitempty" json:"cache_ttl,omitempty"`
	DaemonHost   string `yaml:"daemon_host,omitempty" json:"daemon_host,omitempty"`
	DaemonPort   int    `yaml:"daemon_port,omitempty" json:"daemon_port,omitempty"`
	PostgresDSN  string `yaml:"postgres_dsn,omitempty" json:"postgres_dsn,omitempty"`
}

// Default returns default configuration
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{
			Host:         "127.0.0.1",
			Port:         9002,
			ReadTimeout:  15,
			WriteTimeout: 15,
			IdleTimeout:  60,
			TLSEnabled:   false,
			EnableHTTP2:  true,
		},
		EntryPoint: "https://api.apiproxy.app",
		Cache: CacheConfig{
			Backend:            "sqlite",
			Path:               filepath.Join(home, ".apiproxy", "cache.db"),
			TTL:                86400, // 24 hours
			MemoryCacheEnabled: true,
			MemoryCacheSize:    1000,
			MaxOpenConns:       25,
			MaxIdleConns:       5,
			ConnMaxLifetime:    300,  // 5 minutes
			ConnMaxIdleTime:    60,   // 1 minute
			CleanupInterval:    3600, // 1 hour
		},
		Security: SecurityConfig{
			RateLimitEnabled:      true,
			RateLimitPerIP:        60,  // 60 req/min per IP
			RateLimitPerKey:       300, // 300 req/min per API key
			RateLimitBurst:        10,
			MaxRequestBodySize:    10 * 1024 * 1024,  // 10MB
			MaxResponseBodySize:   50 * 1024 * 1024,  // 50MB
			SSRFProtectionEnabled: true,
			AllowedUpstreamHosts:  []string{"api.apiproxy.app"},
			BlockPrivateIPs:       true,
			MetricsAuthEnabled:    false,
		},
		Client: ClientConfig{
			RequestTimeout:           30,
			DialTimeout:              10,
			KeepAlive:                30,
			MaxIdleConns:             100,
			MaxIdleConnsPerHost:      10,
			MaxConnsPerHost:          100,
			IdleConnTimeout:          90,
			CircuitBreakerEnabled:    true,
			CircuitBreakerThreshold:  5,
			CircuitBreakerTimeout:    60,
			CircuitBreakerHalfOpen:   3,
			DeduplicationEnabled:     true,
		},
		OfflineEndpoints: []string{
			"/health",
			"/status",
		},
		WhitelistedEndpoints: []string{
			"/v1/darkapi/*",
			"/v1/nerdapi/*",
			"/v1/computeapi/*",
		},
	}
}

// Normalize migrates old config format to new format
func (c *Config) Normalize() {
	// Migrate old fields to new structure if present
	if c.Endpoint != "" && c.EntryPoint == "" {
		c.EntryPoint = c.Endpoint
	}
	if c.DaemonHost != "" && c.Server.Host == "" {
		c.Server.Host = c.DaemonHost
	}
	if c.DaemonPort != 0 && c.Server.Port == 0 {
		c.Server.Port = c.DaemonPort
	}
	if c.CacheBackend != "" && c.Cache.Backend == "" {
		c.Cache.Backend = c.CacheBackend
	}
	if c.CachePath != "" && c.Cache.Path == "" {
		c.Cache.Path = c.CachePath
	}
	if c.CacheTTL != 0 && c.Cache.TTL == 0 {
		c.Cache.TTL = c.CacheTTL
	}
	if c.PostgresDSN != "" && c.Cache.PostgresDSN == "" {
		c.Cache.PostgresDSN = c.PostgresDSN
	}

	// Set defaults for server timeouts if not specified
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 15
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 15
	}
}

// Load reads configuration from file (supports both YAML and JSON)
func Load() (*Config, error) {
	// Try config.json first (new format)
	jsonPath := ConfigJSONPath()
	if data, err := os.ReadFile(jsonPath); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config.json: %w", err)
		}
		cfg.Normalize()
		return &cfg, nil
	}

	// Fall back to config.yml (legacy format)
	yamlPath := ConfigPath()
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return Default(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.Normalize()
	return &cfg, nil
}

// LoadJSON loads config from config.json specifically
func LoadJSON() (*Config, error) {
	path := ConfigJSONPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	cfg.Normalize()
	return &cfg, nil
}

// Save writes configuration to file
func Save(cfg *Config) error {
	path := ConfigPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadCredentials loads just the credentials (API key)
func LoadCredentials() (*Config, error) {
	return Load()
}

// SaveCredentials saves credentials securely
func SaveCredentials(cfg *Config) error {
	// Load existing config to preserve other settings
	existing, err := Load()
	if err != nil {
		existing = Default()
	}

	// Update credentials
	existing.APIKey = cfg.APIKey
	existing.Endpoint = cfg.Endpoint
	existing.UserID = cfg.UserID
	existing.Tier = cfg.Tier

	return Save(existing)
}

// ConfigPath returns the path to the YAML config file
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apiproxy", "config.yml")
}

// ConfigJSONPath returns the path to the JSON config file
func ConfigJSONPath() string {
	// Check current directory first
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}

	// Fall back to home directory
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apiproxy", "config.json")
}

// Set updates a configuration value
func (c *Config) Set(key, value string) error {
	switch key {
	case "entry_point", "endpoint":
		c.EntryPoint = value
	case "api_key":
		c.APIKey = value
	case "server.host", "daemon.host", "daemon_host":
		c.Server.Host = value
	case "server.port", "daemon.port", "daemon_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid port value: %s", value)
		}
		c.Server.Port = port
	case "server.read_timeout":
		timeout, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid timeout value: %s", value)
		}
		c.Server.ReadTimeout = timeout
	case "server.write_timeout":
		timeout, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid timeout value: %s", value)
		}
		c.Server.WriteTimeout = timeout
	case "cache.backend", "cache_backend":
		if value != "sqlite" && value != "postgres" {
			return fmt.Errorf("invalid cache backend: %s (must be sqlite or postgres)", value)
		}
		c.Cache.Backend = value
	case "cache.path", "cache_path":
		c.Cache.Path = value
	case "cache.ttl", "cache_ttl":
		ttl, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid TTL value: %s", value)
		}
		c.Cache.TTL = ttl
	case "cache.postgres_dsn", "postgres.dsn", "postgres_dsn":
		c.Cache.PostgresDSN = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// IsEndpointWhitelisted checks if an endpoint is whitelisted
func (c *Config) IsEndpointWhitelisted(endpoint string) bool {
	for _, pattern := range c.WhitelistedEndpoints {
		if matchPattern(pattern, endpoint) {
			return true
		}
	}
	return false
}

// IsEndpointOffline checks if an endpoint supports offline mode
func (c *Config) IsEndpointOffline(endpoint string) bool {
	for _, pattern := range c.OfflineEndpoints {
		if matchPattern(pattern, endpoint) {
			return true
		}
	}
	return false
}

// matchPattern performs simple wildcard matching
func matchPattern(pattern, str string) bool {
	if pattern == str {
		return true
	}

	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(str, prefix)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(str, prefix)
	}

	return false
}

// ToJSON converts config to JSON
func (c *Config) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
