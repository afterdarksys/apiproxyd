package cmd

import (
	"fmt"

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

	switch format {
	case "yaml", "yml":
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "json":
		data, err := cfg.ToJSON()
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

	fmt.Printf("✅ Set %s = %s\n", key, value)
	return nil
}

func initConfigFile() error {
	cfg := config.Default()

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	fmt.Printf("✅ Created default configuration at %s\n", config.ConfigPath())
	return nil
}
