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

var configFileOverride string

// SetPath overrides the configuration file used by Load and Save.
// Passing an empty path restores the default discovery behavior.
func SetPath(path string) {
	configFileOverride = path
}

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
	MaxOpenConns    int `yaml:"max_open_conns" json:"max_open_conns"`         // max open connections
	MaxIdleConns    int `yaml:"max_idle_conns" json:"max_idle_conns"`         // max idle connections
	ConnMaxLifetime int `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`   // seconds
	ConnMaxIdleTime int `yaml:"conn_max_idle_time" json:"conn_max_idle_time"` // seconds
	// Background cleanup
	CleanupInterval int `yaml:"cleanup_interval" json:"cleanup_interval"` // seconds
	// Advanced caching
	StaleIfError          bool `yaml:"stale_if_error" json:"stale_if_error"`
	StaleTTL              int  `yaml:"stale_ttl" json:"stale_ttl"` // seconds
	SemanticDeduplication bool `yaml:"semantic_deduplication" json:"semantic_deduplication"`
}

type PluginConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	Plugins []PluginEntry `yaml:"plugins" json:"plugins"`
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
	// Network exposure. Remote binding is opt-in because local API routes do
	// not provide a complete multi-user authentication boundary.
	AllowRemote bool `yaml:"allow_remote" json:"allow_remote"`
	// Rate limiting
	RateLimitEnabled bool `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	RateLimitPerIP   int  `yaml:"rate_limit_per_ip" json:"rate_limit_per_ip"`   // requests per minute
	RateLimitPerKey  int  `yaml:"rate_limit_per_key" json:"rate_limit_per_key"` // requests per minute
	RateLimitBurst   int  `yaml:"rate_limit_burst" json:"rate_limit_burst"`     // burst size
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
	RequestTimeout int `yaml:"request_timeout" json:"request_timeout"` // seconds
	DialTimeout    int `yaml:"dial_timeout" json:"dial_timeout"`       // seconds
	KeepAlive      int `yaml:"keep_alive" json:"keep_alive"`           // seconds
	// Connection pooling
	MaxIdleConns        int `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int `yaml:"max_conns_per_host" json:"max_conns_per_host"`
	IdleConnTimeout     int `yaml:"idle_conn_timeout" json:"idle_conn_timeout"` // seconds
	// Circuit breaker
	CircuitBreakerEnabled   bool `yaml:"circuit_breaker_enabled" json:"circuit_breaker_enabled"`
	CircuitBreakerThreshold int  `yaml:"circuit_breaker_threshold" json:"circuit_breaker_threshold"` // consecutive failures
	CircuitBreakerTimeout   int  `yaml:"circuit_breaker_timeout" json:"circuit_breaker_timeout"`     // seconds
	CircuitBreakerHalfOpen  int  `yaml:"circuit_breaker_half_open" json:"circuit_breaker_half_open"` // max requests in half-open
	// Request deduplication
	DeduplicationEnabled bool `yaml:"deduplication_enabled" json:"deduplication_enabled"`
}

// QueueConfig holds configuration for the task queue
type QueueConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Backend   string `yaml:"backend" json:"backend"`       // "river" or "asynq"
	RedisAddr string `yaml:"redis_addr" json:"redis_addr"` // Used for asynq
	Workers   int    `yaml:"workers" json:"workers"`
}

// ClusterConfig holds configuration for gRPC clustering
type ClusterConfig struct {
	Enabled   bool     `yaml:"enabled" json:"enabled"`
	NodeID    string   `yaml:"node_id" json:"node_id"`
	Port      int      `yaml:"port" json:"port"`
	Peers     []string `yaml:"peers" json:"peers"`         // List of "host:port" peer addresses
	Broadcast bool     `yaml:"broadcast" json:"broadcast"` // Broadcast invalidations to peers
}

// LLMContextConfig holds configuration for coding-agent context storage.
type LLMContextConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Path               string `yaml:"path" json:"path"`
	MaxRequestBytes    int64  `yaml:"max_request_bytes" json:"max_request_bytes"`
	DefaultPacketBytes int    `yaml:"default_packet_bytes" json:"default_packet_bytes"`
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

	// Queue configuration
	Queue QueueConfig `yaml:"queue,omitempty" json:"queue,omitempty"`

	// Cluster configuration
	Cluster ClusterConfig `yaml:"cluster,omitempty" json:"cluster,omitempty"`

	// LLM context proxy configuration
	LLMContext LLMContextConfig `yaml:"llm_context,omitempty" json:"llm_context,omitempty"`

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
			Backend:               "sqlite",
			Path:                  filepath.Join(home, ".apiproxy", "cache.db"),
			TTL:                   86400, // 24 hours
			MemoryCacheEnabled:    true,
			MemoryCacheSize:       1000,
			MaxOpenConns:          25,
			MaxIdleConns:          5,
			ConnMaxLifetime:       300,  // 5 minutes
			ConnMaxIdleTime:       60,   // 1 minute
			CleanupInterval:       3600, // 1 hour
			StaleIfError:          true,
			StaleTTL:              172800, // 48 hours
			SemanticDeduplication: true,
		},
		Security: SecurityConfig{
			AllowRemote:           false,
			RateLimitEnabled:      true,
			RateLimitPerIP:        60,  // 60 req/min per IP
			RateLimitPerKey:       300, // 300 req/min per API key
			RateLimitBurst:        10,
			MaxRequestBodySize:    10 * 1024 * 1024, // 10MB
			MaxResponseBodySize:   50 * 1024 * 1024, // 50MB
			SSRFProtectionEnabled: true,
			AllowedUpstreamHosts:  []string{"api.apiproxy.app"},
			BlockPrivateIPs:       true,
			MetricsAuthEnabled:    false,
		},
		Client: ClientConfig{
			RequestTimeout:          30,
			DialTimeout:             10,
			KeepAlive:               30,
			MaxIdleConns:            100,
			MaxIdleConnsPerHost:     10,
			MaxConnsPerHost:         100,
			IdleConnTimeout:         90,
			CircuitBreakerEnabled:   true,
			CircuitBreakerThreshold: 5,
			CircuitBreakerTimeout:   60,
			CircuitBreakerHalfOpen:  3,
			DeduplicationEnabled:    true,
		},
		Queue: QueueConfig{
			Enabled:   false,
			Backend:   "river",
			RedisAddr: "localhost:6379",
			Workers:   10,
		},
		Cluster: ClusterConfig{
			Enabled:   false,
			NodeID:    "node-1",
			Port:      9005,
			Peers:     []string{},
			Broadcast: true,
		},
		LLMContext: LLMContextConfig{
			Enabled:            false,
			Path:               filepath.Join(home, ".apiproxy", "llm_context.db"),
			MaxRequestBytes:    10 * 1024 * 1024,
			DefaultPacketBytes: 12000,
		},
		OfflineEndpoints: []string{
			"/health",
			"/status",
		},
		WhitelistedEndpoints: []string{
			"/darkapi/*",
			"/dnsscience/*",
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
	if c.LLMContext.Path == "" {
		home, _ := os.UserHomeDir()
		c.LLMContext.Path = filepath.Join(home, ".apiproxy", "llm_context.db")
	}
	if c.LLMContext.MaxRequestBytes == 0 {
		c.LLMContext.MaxRequestBytes = 10 * 1024 * 1024
	}
	if c.LLMContext.DefaultPacketBytes == 0 {
		c.LLMContext.DefaultPacketBytes = 12000
	}

	c.Cache.Path = expandHome(c.Cache.Path)
	c.LLMContext.Path = expandHome(c.LLMContext.Path)
	for i := range c.Plugins.Plugins {
		c.Plugins.Plugins[i].Path = expandHome(c.Plugins.Plugins[i].Path)
	}
}

// Load reads configuration from file (supports both YAML and JSON)
func Load() (*Config, error) {
	if configFileOverride != "" {
		return loadFile(configFileOverride)
	}

	// Try config.json first (new format)
	jsonPath := ConfigJSONPath()
	if _, err := os.Stat(jsonPath); err == nil {
		return loadFile(jsonPath)
	}

	// Fall back to config.yml (legacy format)
	yamlPath := ConfigPath()
	if _, err := os.Stat(yamlPath); err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			cfg := Default()
			if err := applyEnvironment(cfg); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return loadFile(yamlPath)
}

// LoadJSON loads config from config.json specifically
func LoadJSON() (*Config, error) {
	return loadFile(ConfigJSONPath())
}

// Save writes configuration to file
func Save(cfg *Config) error {
	path := configFileOverride
	if path == "" {
		jsonPath := ConfigJSONPath()
		if _, err := os.Stat(jsonPath); err == nil {
			path = jsonPath
		} else {
			path = ConfigPath()
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var data []byte
	var err error
	if strings.EqualFold(filepath.Ext(path), ".json") {
		data, err = json.MarshalIndent(cfg, "", "  ")
		data = append(data, '\n')
	} else {
		data, err = yaml.Marshal(cfg)
	}
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
	if cfg.EntryPoint != "" {
		existing.EntryPoint = cfg.EntryPoint
	} else if cfg.Endpoint != "" {
		existing.EntryPoint = cfg.Endpoint
	}
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

func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	cfg := Default()
	provided := &Config{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, cfg)
		if err == nil {
			err = yaml.Unmarshal(data, provided)
		}
	default:
		err = json.Unmarshal(data, cfg)
		if err == nil {
			err = json.Unmarshal(data, provided)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	applyLegacyFields(cfg, provided)
	cfg.Normalize()
	if err := applyEnvironment(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyLegacyFields(cfg, provided *Config) {
	if provided.Endpoint != "" && provided.EntryPoint == "" {
		cfg.EntryPoint = provided.Endpoint
	}
	if provided.DaemonHost != "" && provided.Server.Host == "" {
		cfg.Server.Host = provided.DaemonHost
	}
	if provided.DaemonPort != 0 && provided.Server.Port == 0 {
		cfg.Server.Port = provided.DaemonPort
	}
	if provided.CacheBackend != "" && provided.Cache.Backend == "" {
		cfg.Cache.Backend = provided.CacheBackend
	}
	if provided.CachePath != "" && provided.Cache.Path == "" {
		cfg.Cache.Path = provided.CachePath
	}
	if provided.CacheTTL != 0 && provided.Cache.TTL == 0 {
		cfg.Cache.TTL = provided.CacheTTL
	}
	if provided.PostgresDSN != "" && provided.Cache.PostgresDSN == "" {
		cfg.Cache.PostgresDSN = provided.PostgresDSN
	}
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func applyEnvironment(cfg *Config) error {
	keys := map[string]string{
		"APIPROXY_API_KEY":               "api_key",
		"APIPROXY_ENTRY_POINT":           "entry_point",
		"APIPROXY_SERVER_HOST":           "server.host",
		"APIPROXY_SERVER_PORT":           "server.port",
		"APIPROXY_CACHE_BACKEND":         "cache.backend",
		"APIPROXY_CACHE_PATH":            "cache.path",
		"APIPROXY_CACHE_TTL":             "cache.ttl",
		"APIPROXY_CACHE_POSTGRES_DSN":    "cache.postgres_dsn",
		"APIPROXY_SECURITY_ALLOW_REMOTE": "security.allow_remote",
		"APIPROXY_LLM_CONTEXT_ENABLED":   "llm_context.enabled",
		"APIPROXY_LLM_CONTEXT_PATH":      "llm_context.path",
	}
	for envKey, configKey := range keys {
		if value, ok := os.LookupEnv(envKey); ok {
			if err := cfg.Set(configKey, value); err != nil {
				return fmt.Errorf("invalid %s: %w", envKey, err)
			}
		}
	}
	cfg.Normalize()
	return nil
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
	case "security.allow_remote":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid security.allow_remote value: %s", value)
		}
		c.Security.AllowRemote = enabled
	case "llm_context.enabled":
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid llm_context.enabled value: %s", value)
		}
		c.LLMContext.Enabled = enabled
	case "llm_context.path":
		c.LLMContext.Path = value
	case "llm_context.max_request_bytes":
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid llm_context.max_request_bytes value: %s", value)
		}
		c.LLMContext.MaxRequestBytes = limit
	case "llm_context.default_packet_bytes":
		limit, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid llm_context.default_packet_bytes value: %s", value)
		}
		c.LLMContext.DefaultPacketBytes = limit
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
