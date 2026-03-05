package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// APIKeysConfig represents the structure of api_keys.json
type APIKeysConfig struct {
	Keys       []string `json:"keys"`
	OAuthToken string   `json:"oauth_token,omitempty"`
}

// APIKeysJSONPath returns the path to the api_keys.json file
func APIKeysJSONPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apiproxy", "api_keys.json")
}

// LoadAPIKeys reads the api_keys.json file
func LoadAPIKeys() (*APIKeysConfig, error) {
	path := APIKeysJSONPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &APIKeysConfig{Keys: []string{}}, nil
		}
		return nil, fmt.Errorf("failed to read api_keys.json: %w", err)
	}

	var cfg APIKeysConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse api_keys.json: %w", err)
	}

	return &cfg, nil
}

// SaveAPIKeys writes the API keys back to api_keys.json
func SaveAPIKeys(cfg *APIKeysConfig) error {
	path := APIKeysJSONPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal api_keys.json: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write api_keys.json: %w", err)
	}

	return nil
}

// AddKey adds a new API key to the configuration
func (c *APIKeysConfig) AddKey(key string) {
	// Don't add if it already exists
	for _, existing := range c.Keys {
		if existing == key {
			return
		}
	}
	c.Keys = append(c.Keys, key)
}

// GetPrimaryAuth returns the primary authentication method to use
// It returns (value, type(Bearer|APIKey))
func (c *APIKeysConfig) GetPrimaryAuth() (string, string) {
	if c.OAuthToken != "" {
		return c.OAuthToken, "Bearer"
	}
	if len(c.Keys) > 0 {
		return c.Keys[0], "APIKey"
	}
	return "", ""
}
