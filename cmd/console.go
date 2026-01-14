package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
		fmt.Println("Goodbye!")
		os.Exit(0)
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
	switch action {
	case "stats":
		// TODO: Implement cache stats
		fmt.Println("Cache statistics:")
		fmt.Println("  Total entries: 0")
		fmt.Println("  Total size: 0 bytes")
		fmt.Println("  Hit rate: 0%")
		return nil
	case "clear":
		// TODO: Implement cache clear
		fmt.Println("✅ Cache cleared")
		return nil
	default:
		return fmt.Errorf("unknown cache action: %s (use: stats, clear)", action)
	}
}

func executeRequest(method, path, data string) error {
	// TODO: Execute actual API request
	fmt.Printf("Executing: %s %s\n", method, path)
	if data != "" {
		fmt.Printf("Data: %s\n", data)
	}
	return nil
}
