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
}

type CacheConfig struct {
	Backend     string `yaml:"backend" json:"backend"`
	Path        string `yaml:"path" json:"path"`
	TTL         int    `yaml:"ttl" json:"ttl"` // seconds
	PostgresDSN string `yaml:"postgres_dsn,omitempty" json:"postgres_dsn,omitempty"`
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
		},
		EntryPoint: "https://api.apiproxy.app",
		Cache: CacheConfig{
			Backend: "sqlite",
			Path:    filepath.Join(home, ".apiproxy", "cache.db"),
			TTL:     86400, // 24 hours
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
