package cmd

import (
	"fmt"

	"github.com/afterdarktech/apiproxyd/pkg/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon [start|stop|status|restart]",
	Short: "Manage the background daemon service",
	Long: `Control the apiproxyd background service.

The daemon runs a local HTTP proxy server that caches API requests
and responses, reducing latency and costs.

Examples:
  apiproxy daemon start            # Start daemon in background
  apiproxy daemon stop             # Stop daemon
  apiproxy daemon status           # Check daemon status
  apiproxy daemon restart          # Restart daemon`,
	Args: cobra.ExactArgs(1),
	RunE: runDaemon,
}

var (
	daemonPort int
	daemonHost string
)

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().IntVarP(&daemonPort, "port", "p", 9002, "daemon listen port")
	daemonCmd.Flags().StringVar(&daemonHost, "host", "127.0.0.1", "daemon listen host")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	action := args[0]

	d := daemon.New(daemonHost, daemonPort)

	switch action {
	case "start":
		fmt.Printf("Starting apiproxyd daemon on %s:%d...\n", daemonHost, daemonPort)
		return d.Start()
	case "stop":
		fmt.Println("Stopping apiproxyd daemon...")
		return d.Stop()
	case "status":
		return d.Status()
	case "restart":
		fmt.Println("Restarting apiproxyd daemon...")
		if err := d.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop daemon: %v\n", err)
		}
		return d.Start()
	default:
		return fmt.Errorf("unknown action: %s (use: start, stop, status, restart)", action)
	}
}
