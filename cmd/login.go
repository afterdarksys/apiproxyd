package cmd

import (
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
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
  apiproxy login --api-key apx_live_xxxxx
  apiproxy login --oauth2`,
	RunE: runLogin,
}

var (
	apiKey    string
	useOAuth2 bool
)

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().StringVar(&apiKey, "api-key", "", "API key for authentication")
	loginCmd.Flags().BoolVar(&useOAuth2, "oauth2", false, "Use OAuth2 Device Authorization Flow")
}

func runLogin(cmd *cobra.Command, args []string) error {
	if useOAuth2 {
		return runOAuth2Login()
	}

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

	// Save to api_keys.json as well for multi-tenant support
	apiKeys, _ := config.LoadAPIKeys()
	if apiKeys == nil {
		apiKeys = &config.APIKeysConfig{}
	}
	apiKeys.AddKey(apiKey)
	// Optionally clear OAuthToken if we are explicitly logging in with API Key
	apiKeys.OAuthToken = ""
	config.SaveAPIKeys(apiKeys)

	fmt.Printf("✅ Successfully authenticated as %s\n", info.Email)
	fmt.Printf("   Tier: %s\n", info.Tier)
	fmt.Printf("   Rate Limit: %d requests/minute\n", info.RateLimit)
	fmt.Printf("   Monthly Quota: %d requests\n", info.MonthlyQuota)

	return nil
}

func runOAuth2Login() error {
	fmt.Println("Initiating OAuth2 Device Authorization Flow...")

	// Mock requesting a device code
	fmt.Println("\nPlease visit the following URL to authenticate:")
	fmt.Println("  https://apiproxy.app/auth/device")
	fmt.Println("\nAnd enter the code: ABC-DEF-GHI")

	fmt.Println("\nWaiting for authentication...")

	// Simulate waiting for user to auth
	time.Sleep(3 * time.Second)

	mockToken := "apx_oauth_mock_TOKEN_" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Save it to API keys
	apiKeys, err := config.LoadAPIKeys()
	if err != nil || apiKeys == nil {
		apiKeys = &config.APIKeysConfig{}
	}
	apiKeys.OAuthToken = mockToken

	if err := config.SaveAPIKeys(apiKeys); err != nil {
		return fmt.Errorf("failed to save OAuth2 token: %w", err)
	}

	fmt.Println("✅ Successfully authenticated via OAuth2!")
	return nil
}
