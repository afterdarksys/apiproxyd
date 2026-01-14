package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run diagnostic tests",
	Long: `Run diagnostic tests to verify apiproxyd setup.

Tests:
  - Authentication with api.apiproxy.app
  - Cache read/write operations
  - Daemon connectivity
  - Configuration validity

Example:
  apiproxy test                # Run all tests
  apiproxy test --verbose      # Show detailed output`,
	RunE: runTest,
}

var testVerbose bool

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "verbose output")
}

func runTest(cmd *cobra.Command, args []string) error {
	fmt.Println("Running apiproxyd diagnostic tests...\n")

	results := make(map[string]bool)

	// Test 1: Configuration
	fmt.Print("1. Testing configuration... ")
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("❌ FAILED")
		if testVerbose {
			fmt.Printf("   Error: %v\n", err)
		}
		results["config"] = false
	} else {
		fmt.Println("✅ PASSED")
		if testVerbose {
			fmt.Printf("   Endpoint: %s\n", cfg.Endpoint)
			fmt.Printf("   Cache: %s (%s)\n", cfg.CacheBackend, cfg.CachePath)
		}
		results["config"] = true
	}

	// Test 2: Authentication
	fmt.Print("2. Testing authentication... ")
	if cfg != nil && cfg.APIKey != "" {
		c := client.New(cfg.APIKey)
		info, err := c.ValidateKey()
		if err != nil {
			fmt.Println("❌ FAILED")
			if testVerbose {
				fmt.Printf("   Error: %v\n", err)
			}
			results["auth"] = false
		} else {
			fmt.Println("✅ PASSED")
			if testVerbose {
				fmt.Printf("   Email: %s\n", info.Email)
				fmt.Printf("   Tier: %s\n", info.Tier)
			}
			results["auth"] = true
		}
	} else {
		fmt.Println("⏭️  SKIPPED (not authenticated)")
		results["auth"] = false
	}

	// Test 3: Cache operations
	fmt.Print("3. Testing cache... ")
	if cfg != nil {
		cacheStore, err := cache.New(cfg.CacheBackend, cfg.CachePath)
		if err != nil {
			fmt.Println("❌ FAILED")
			if testVerbose {
				fmt.Printf("   Error: %v\n", err)
			}
			results["cache"] = false
		} else {
			defer cacheStore.Close()

			// Test write
			testKey := fmt.Sprintf("test:%d", time.Now().Unix())
			testData := []byte(`{"test": "data"}`)

			if err := cacheStore.Set(testKey, testData); err != nil {
				fmt.Println("❌ FAILED (write)")
				if testVerbose {
					fmt.Printf("   Error: %v\n", err)
				}
				results["cache"] = false
			} else {
				// Test read
				retrieved, err := cacheStore.Get(testKey)
				if err != nil || string(retrieved) != string(testData) {
					fmt.Println("❌ FAILED (read)")
					if testVerbose {
						fmt.Printf("   Error: %v\n", err)
					}
					results["cache"] = false
				} else {
					fmt.Println("✅ PASSED")
					if testVerbose {
						stats, _ := cacheStore.Stats()
						fmt.Printf("   Backend: %s\n", cfg.CacheBackend)
						fmt.Printf("   Entries: %d\n", stats.Entries)
					}
					results["cache"] = true
				}

				// Cleanup
				cacheStore.Delete(testKey)
			}
		}
	} else {
		fmt.Println("⏭️  SKIPPED (no config)")
		results["cache"] = false
	}

	// Test 4: Daemon connectivity (optional)
	fmt.Print("4. Testing daemon... ")
	// TODO: Implement daemon connectivity test
	fmt.Println("⏭️  SKIPPED (not implemented)")
	results["daemon"] = false

	// Summary
	fmt.Println("\n" + strings.Repeat("-", 40))
	passed := 0
	total := 0
	for _, result := range results {
		total++
		if result {
			passed++
		}
	}

	fmt.Printf("Tests: %d passed, %d failed, %d total\n", passed, total-passed, total)

	if passed == total {
		fmt.Println("✅ All tests passed!")
		return nil
	} else if passed > 0 {
		fmt.Println("⚠️  Some tests failed")
		return nil
	} else {
		return fmt.Errorf("all tests failed")
	}
}
