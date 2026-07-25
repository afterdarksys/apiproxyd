package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Interactive console for API testing",
	Long: `Launch an interactive console for testing API requests.

The console provides a REPL environment for making requests,
viewing cache status, and debugging API interactions.

Example:
  apiproxy console

Console commands:
  GET /v1/darkapi/ip/8.8.8.8     # Make GET request
  POST /v1/nerdapi/hash {"data"}  # Make POST request
  cache stats                     # View cache statistics
  cache clear                     # Clear cache
  help                            # Show help
  exit                            # Exit console`,
	RunE: runConsole,
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}

func runConsole(cmd *cobra.Command, args []string) error {
	fmt.Println("apiproxyd interactive console")
	fmt.Println("Type 'help' for commands, 'exit' to quit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("apiproxy> ")

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
			fmt.Println("Goodbye")
			break
		}

		if err := handleConsoleCommand(line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("console error: %w", err)
	}

	return nil
}

func handleConsoleCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "exit", "quit":
		return nil
	case "help", "?":
		printConsoleHelp()
	case "cache":
		if len(parts) < 2 {
			return fmt.Errorf("usage: cache [stats|clear]")
		}
		return handleCacheCommand(parts[1])
	case "get", "post", "put", "delete", "patch":
		if len(parts) < 2 {
			return fmt.Errorf("usage: %s <path> [data]", strings.ToUpper(cmd))
		}
		path := parts[1]
		data := ""
		if len(parts) > 2 {
			data = strings.Join(parts[2:], " ")
		}
		return executeRequest(strings.ToUpper(cmd), path, data)
	default:
		return fmt.Errorf("unknown command: %s (type 'help' for available commands)", cmd)
	}

	return nil
}

func printConsoleHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  GET <path>              Make GET request")
	fmt.Println("  POST <path> <data>      Make POST request")
	fmt.Println("  PUT <path> <data>       Make PUT request")
	fmt.Println("  DELETE <path>           Make DELETE request")
	fmt.Println("  cache stats             Show cache statistics")
	fmt.Println("  cache clear             Clear all cached data")
	fmt.Println("  help                    Show this help")
	fmt.Println("  exit                    Exit console")
}

func handleCacheCommand(action string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cacheStore, err := newCacheFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("open cache: %w", err)
	}
	defer cacheStore.Close()

	switch action {
	case "stats":
		stats, err := cacheStore.Stats()
		if err != nil {
			return fmt.Errorf("read cache stats: %w", err)
		}
		fmt.Printf("Entries: %d\n", stats.Entries)
		fmt.Printf("Size: %d bytes\n", stats.SizeBytes)
		fmt.Printf("Hit rate: %.2f%%\n", stats.HitRate*100)
		return nil
	case "clear":
		clearer, ok := cacheStore.(interface{ Clear() error })
		if !ok {
			return fmt.Errorf("cache backend does not support clearing")
		}
		if err := clearer.Clear(); err != nil {
			return fmt.Errorf("clear cache: %w", err)
		}
		fmt.Println("Cache cleared")
		return nil
	default:
		return fmt.Errorf("unknown cache action: %s (use: stats, clear)", action)
	}
}

func executeRequest(method, path, data string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cacheStore, err := newCacheFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("open cache: %w", err)
	}
	defer cacheStore.Close()

	cacheKey := cache.GenerateKey(method, path, data)
	if cache.IsCacheableMethod(method) {
		if cached, err := cacheStore.Get(cacheKey); err == nil {
			printResponse(cached)
			return nil
		}
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("not authenticated; run 'apiproxy login' first")
	}

	apiClient := client.New(cfg.APIKey)
	apiClient.BaseURL = cfg.EntryPoint
	response, err := apiClient.Request(method, path, strings.NewReader(data), nil)
	if err != nil {
		return err
	}
	if cache.IsCacheableMethod(method) {
		if err := cacheStore.Set(cacheKey, response); err != nil {
			return fmt.Errorf("cache response: %w", err)
		}
	}
	printResponse(response)
	return nil
}
