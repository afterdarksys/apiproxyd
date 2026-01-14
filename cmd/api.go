package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var apiCmd = &cobra.Command{
	Use:   "api [METHOD] [PATH]",
	Short: "Make API requests through the cache",
	Long: `Make API requests to api.apiproxy.app with automatic caching.

The daemon will:
  1. Check local cache for identical request
  2. Return cached response if valid
  3. Otherwise, make request to api.apiproxy.app
  4. Cache the response for future requests

Examples:
  apiproxy api GET /v1/darkapi/ip/8.8.8.8
  apiproxy api POST /v1/nerdapi/hash --data '{"value":"test","algorithm":"sha256"}'
  apiproxy api GET /v1/status --no-cache`,
	Args: cobra.MinimumNArgs(2),
	RunE: runAPI,
}

var (
	apiData    string
	apiHeaders []string
	noCache    bool
	cacheOnly  bool
)

func init() {
	rootCmd.AddCommand(apiCmd)
	apiCmd.Flags().StringVarP(&apiData, "data", "d", "", "request body (JSON)")
	apiCmd.Flags().StringArrayVarP(&apiHeaders, "header", "H", []string{}, "custom headers (key:value)")
	apiCmd.Flags().BoolVar(&noCache, "no-cache", false, "bypass cache and force fresh request")
	apiCmd.Flags().BoolVar(&cacheOnly, "cache-only", false, "only return cached response, don't make request")
}

func runAPI(cmd *cobra.Command, args []string) error {
	method := strings.ToUpper(args[0])
	path := args[1]

	// Load credentials
	cfg, err := config.LoadCredentials()
	if err != nil {
		return fmt.Errorf("not authenticated. Run 'apiproxy login' first")
	}

	// Initialize cache
	cacheStore, err := cache.New(cfg.CacheBackend, cfg.CachePath)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	defer cacheStore.Close()

	// Build request
	var body io.Reader
	if apiData != "" {
		body = strings.NewReader(apiData)
	}

	headers := make(map[string]string)
	for _, h := range apiHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Generate cache key
	cacheKey := cache.GenerateKey(method, path, apiData)

	// Try cache first (unless no-cache flag)
	if !noCache {
		if cached, err := cacheStore.Get(cacheKey); err == nil {
			if viper.GetBool("debug") {
				fmt.Fprintln(os.Stderr, "✅ Cache hit")
			}
			printResponse(cached)
			return nil
		}
	}

	// If cache-only mode and not in cache, return error
	if cacheOnly {
		return fmt.Errorf("not found in cache (use --no-cache to fetch)")
	}

	// Make request
	c := client.New(cfg.APIKey)
	resp, err := c.Request(method, path, body, headers)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	// Cache the response
	if err := cacheStore.Set(cacheKey, resp); err != nil {
		if viper.GetBool("debug") {
			fmt.Fprintf(os.Stderr, "Warning: failed to cache response: %v\n", err)
		}
	}

	// Print response
	printResponse(resp)
	return nil
}

func printResponse(data []byte) {
	format := viper.GetString("format")

	switch format {
	case "yaml", "yml":
		var obj interface{}
		if err := json.Unmarshal(data, &obj); err == nil {
			if out, err := yaml.Marshal(obj); err == nil {
				fmt.Println(string(out))
				return
			}
		}
		fmt.Println(string(data))
	case "json":
		fallthrough
	default:
		// Pretty print JSON
		var obj interface{}
		if err := json.Unmarshal(data, &obj); err == nil {
			if out, err := json.MarshalIndent(obj, "", "  "); err == nil {
				fmt.Println(string(out))
				return
			}
		}
		fmt.Println(string(data))
	}
}
