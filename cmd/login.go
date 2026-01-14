package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/afterdarktech/apiproxyd/pkg/client"
	"github.com/afterdarktech/apiproxyd/pkg/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with api.apiproxy.app",
	Long: `Login to api.apiproxy.app and store authentication token locally.

The token is stored securely in ~/.apiproxy/credentials and used
for all subsequent API requests.

Example:
  apiproxy login
  apiproxy login --api-key apx_live_xxxxx`,
	RunE: runLogin,
}

var (
	apiKey string
)

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().StringVar(&apiKey, "api-key", "", "API key for authentication")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// If API key not provided via flag, prompt for it
	if apiKey == "" {
		fmt.Print("Enter your API key: ")
		keyBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read API key: %w", err)
		}
		fmt.Println()
		apiKey = strings.TrimSpace(string(keyBytes))
	}

	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Validate API key format
	if !strings.HasPrefix(apiKey, "apx_live_") && !strings.HasPrefix(apiKey, "apx_test_") {
		return fmt.Errorf("invalid API key format (expected apx_live_* or apx_test_*)")
	}

	// Test the API key by making a request to the API
	c := client.New(apiKey)
	info, err := c.ValidateKey()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	cfg := &config.Config{
		APIKey:   apiKey,
		Endpoint: c.BaseURL,
		UserID:   info.UserID,
		Tier:     info.Tier,
	}

	if err := config.SaveCredentials(cfg); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Printf("✅ Successfully authenticated as %s\n", info.Email)
	fmt.Printf("   Tier: %s\n", info.Tier)
	fmt.Printf("   Rate Limit: %d requests/minute\n", info.RateLimit)
	fmt.Printf("   Monthly Quota: %d requests\n", info.MonthlyQuota)

	return nil
}
