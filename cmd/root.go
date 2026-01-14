package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile   string
	Version   string
	Commit    string
	BuildDate string
)

var rootCmd = &cobra.Command{
	Use:   "apiproxy",
	Short: "API Proxy Cache Daemon for On-Site Deployment",
	Long: `apiproxyd is a companion daemon for api.apiproxy.app that provides
on-premises API caching, reducing costs and improving performance.

Caches:
  - API requests and responses
  - Question metadata
  - Question categories
  - Authentication tokens

Example usage:
  apiproxy login                    # Authenticate with api.apiproxy.app
  apiproxy api GET /v1/status       # Make API request through cache
  apiproxy daemon start             # Start background daemon
  apiproxy config show              # Display configuration`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, BuildDate),
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.apiproxy/config.yml)")
	rootCmd.PersistentFlags().String("format", "json", "output format: json, yaml, table")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")

	viper.BindPFlag("format", rootCmd.PersistentFlags().Lookup("format"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home + "/.apiproxy")
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("APIPROXY")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("debug") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
