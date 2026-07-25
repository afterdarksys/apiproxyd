package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config [show|set|init]",
	Short: "Manage configuration",
	Long: `View and modify apiproxyd configuration.

Configuration is stored in ~/.apiproxy/config.yml

Examples:
  apiproxy config show                          # Display current config
  apiproxy config set cache.backend sqlite      # Set cache backend
  apiproxy config set cache.ttl 3600            # Set cache TTL (seconds)
  apiproxy config init                          # Initialize default config`,
	Args: cobra.MinimumNArgs(1),
	RunE: runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	action := args[0]

	switch action {
	case "show":
		return showConfig()
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: apiproxy config set <key> <value>")
		}
		return setConfig(args[1], args[2])
	case "init":
		return initConfigFile()
	default:
		return fmt.Errorf("unknown action: %s (use: show, set, init)", action)
	}
}

func showConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	format := viper.GetString("format")
	displayCfg := redactConfig(cfg)

	switch format {
	case "yaml", "yml":
		data, err := yaml.Marshal(displayCfg)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "json":
		data, err := displayCfg.ToJSON()
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	default:
		fmt.Printf("Entry Point: %s\n", cfg.EntryPoint)
		fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
		fmt.Printf("Cache Backend: %s\n", cfg.Cache.Backend)
		fmt.Printf("Cache Path: %s\n", cfg.Cache.Path)
		fmt.Printf("Cache TTL: %d seconds\n", cfg.Cache.TTL)
		if len(cfg.WhitelistedEndpoints) > 0 {
			fmt.Printf("Whitelisted Endpoints: %d\n", len(cfg.WhitelistedEndpoints))
		}
		if len(cfg.OfflineEndpoints) > 0 {
			fmt.Printf("Offline Endpoints: %d\n", len(cfg.OfflineEndpoints))
		}
		if cfg.UserID != "" {
			fmt.Printf("User ID: %s\n", cfg.UserID)
			fmt.Printf("Tier: %s\n", cfg.Tier)
		}
	}

	return nil
}

func redactConfig(cfg *config.Config) *config.Config {
	data, err := json.Marshal(cfg)
	if err != nil {
		return cfg
	}
	var redacted config.Config
	if err := json.Unmarshal(data, &redacted); err != nil {
		return cfg
	}
	if redacted.APIKey != "" {
		redacted.APIKey = "[REDACTED]"
	}
	if redacted.Cache.PostgresDSN != "" {
		redacted.Cache.PostgresDSN = "[REDACTED]"
	}
	if redacted.Security.MetricsAuthToken != "" {
		redacted.Security.MetricsAuthToken = "[REDACTED]"
	}
	for i := range redacted.Plugins.Plugins {
		redactMap(redacted.Plugins.Plugins[i].Config)
	}
	return &redacted
}

func redactMap(values map[string]interface{}) {
	for key, value := range values {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "key") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "password") ||
			strings.Contains(lowerKey, "secret") {
			values[key] = "[REDACTED]"
			continue
		}
		if nested, ok := value.(map[string]interface{}); ok {
			redactMap(nested)
		}
	}
}

func setConfig(key, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Set(key, value); err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

func initConfigFile() error {
	cfg := config.Default()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	fmt.Printf("Created default configuration at %s\n", config.ConfigPath())
	return nil
}
