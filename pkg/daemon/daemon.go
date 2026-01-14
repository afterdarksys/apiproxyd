package daemon

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/afterdarksys/apiproxyd/pkg/metrics"
	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

type Daemon struct {
	host          string
	port          int
	server        *http.Server
	cache         cache.Cache
	client        *client.Client
	cfg           *config.Config
	pluginManager *plugin.Manager
	metrics       *metrics.PrometheusMetrics
}

func New(host string, port int) *Daemon {
	return &Daemon{
		host: host,
		port: port,
	}
}

func (d *Daemon) Start() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	d.cfg = cfg

	// Override host/port if provided
	if d.host == "" || d.host == "127.0.0.1" {
		d.host = cfg.Server.Host
	}
	if d.port == 0 || d.port == 9002 {
		d.port = cfg.Server.Port
	}

	// Initialize cache
	cachePath := cfg.Cache.Path
	if cfg.Cache.Backend == "postgres" {
		cachePath = cfg.Cache.PostgresDSN
	}

	cacheStore, err := cache.New(cfg.Cache.Backend, cachePath)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	d.cache = cacheStore

	// Initialize client
	if cfg.APIKey != "" {
		d.client = client.New(cfg.APIKey)
		d.client.BaseURL = cfg.EntryPoint
	}

	// Initialize plugin manager
	pluginCfg := &plugin.Config{
		Enabled: cfg.Plugins.Enabled,
		Plugins: make([]plugin.PluginConfig, len(cfg.Plugins.Plugins)),
	}
	for i, pe := range cfg.Plugins.Plugins {
		pluginCfg.Plugins[i] = plugin.PluginConfig{
			Name:    pe.Name,
			Type:    pe.Type,
			Path:    pe.Path,
			Enabled: pe.Enabled,
			Config:  pe.Config,
		}
	}
	d.pluginManager = plugin.NewManager(pluginCfg)
	if err := d.pluginManager.LoadPlugins(); err != nil {
		return fmt.Errorf("failed to load plugins: %w", err)
	}

	// Initialize metrics
	d.metrics = metrics.NewPrometheusMetrics()

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/api/", d.handleProxy)
	mux.HandleFunc("/cache/stats", d.handleCacheStats)
	mux.HandleFunc("/cache/clear", d.handleCacheClear)
	mux.HandleFunc("/metrics", d.metrics.ServeHTTP)

	d.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", d.host, d.port),
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Start server in background
	go func() {
		fmt.Printf("✅ Daemon started on %s:%d\n", d.host, d.port)
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down daemon...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	d.cache.Close()
	if d.pluginManager != nil {
		d.pluginManager.Shutdown()
	}
	d.removePIDFile()

	fmt.Println("✅ Daemon stopped")
	return nil
}

func (d *Daemon) Stop() error {
	pidPath := d.pidFilePath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon is not running")
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	os.Remove(pidPath)
	fmt.Println("✅ Daemon stopped")
	return nil
}

func (d *Daemon) Status() error {
	pidPath := d.pidFilePath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("❌ Daemon is not running")
			return nil
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("❌ Daemon is not running")
		return nil
	}

	// Check if process is actually running
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("❌ Daemon is not running (stale PID file)")
		os.Remove(pidPath)
		return nil
	}

	fmt.Printf("✅ Daemon is running (PID: %d)\n", pid)

	// Try to get health status
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", d.host, d.port))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("   Endpoint: http://%s:%d\n", d.host, d.port)
		}
	}

	return nil
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "0.1.0",
		"uptime":  time.Since(time.Now()).Seconds(),
	})
}

func (d *Daemon) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	ctx := r.Context()

	// Extract endpoint path (remove /api prefix)
	endpoint := strings.TrimPrefix(r.URL.Path, "/api")

	// Check if endpoint is whitelisted
	if !d.cfg.IsEndpointWhitelisted(endpoint) {
		http.Error(w, fmt.Sprintf("Endpoint not whitelisted: %s", endpoint), http.StatusForbidden)
		return
	}

	// Read body
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// Create plugin request
	pluginReq := plugin.FromHTTPRequest(r, body)
	pluginReq.Endpoint = endpoint

	// Call plugin OnRequest hooks
	if d.pluginManager != nil {
		modifiedReq, cont, err := d.pluginManager.OnRequest(ctx, pluginReq)
		if err != nil {
			http.Error(w, fmt.Sprintf("Plugin error: %v", err), http.StatusInternalServerError)
			return
		}
		if !cont {
			// Plugin stopped the request, return early
			w.WriteHeader(http.StatusOK)
			return
		}
		pluginReq = modifiedReq
		// Update endpoint and body in case plugins modified them
		endpoint = pluginReq.Endpoint
		body = pluginReq.Body
	}

	// Generate cache key
	cacheKey := cache.GenerateKey(pluginReq.Method, endpoint, string(body))

	// Check if this is an offline endpoint
	isOffline := d.cfg.IsEndpointOffline(endpoint)

	// Try cache first
	if cached, err := d.cache.Get(cacheKey); err == nil {
		pluginResp := &plugin.Response{
			StatusCode: http.StatusOK,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       cached,
			Cached:     true,
		}

		// Call plugin OnCacheHit hooks
		if d.pluginManager != nil {
			modifiedResp, err := d.pluginManager.OnCacheHit(ctx, pluginReq, pluginResp)
			if err != nil {
				http.Error(w, fmt.Sprintf("Plugin error: %v", err), http.StatusInternalServerError)
				return
			}
			pluginResp = modifiedResp
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		if isOffline {
			w.Header().Set("X-Offline", "true")
		}
		for k, v := range pluginResp.Headers {
			w.Header().Set(k, v)
		}
		d.writeResponse(w, r, pluginResp.Body, startTime, true)
		d.metrics.RecordRequest(r.Method, http.StatusOK, time.Since(startTime), true, int64(len(pluginResp.Body)))
		return
	}

	// If offline endpoint and not in cache, return error
	if isOffline {
		http.Error(w, "Offline endpoint not available in cache", http.StatusServiceUnavailable)
		return
	}

	// Check authentication for online requests
	if d.client == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Make request to API
	headers := make(map[string]string)
	for k, v := range pluginReq.Headers {
		headers[k] = v
	}

	resp, err := d.client.Request(pluginReq.Method, endpoint, bytes.NewReader(body), headers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Create plugin response
	pluginResp := &plugin.Response{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       resp,
		Cached:     false,
	}

	// Call plugin OnResponse hooks
	if d.pluginManager != nil {
		modifiedResp, err := d.pluginManager.OnResponse(ctx, pluginReq, pluginResp)
		if err != nil {
			http.Error(w, fmt.Sprintf("Plugin error: %v", err), http.StatusInternalServerError)
			return
		}
		pluginResp = modifiedResp
	}

	// Cache response (with longer TTL for offline endpoints)
	d.cache.Set(cacheKey, pluginResp.Body)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	for k, v := range pluginResp.Headers {
		w.Header().Set(k, v)
	}
	d.writeResponse(w, r, pluginResp.Body, startTime, false)
	d.metrics.RecordRequest(r.Method, http.StatusOK, time.Since(startTime), false, int64(len(pluginResp.Body)))
}

// writeResponse writes response with optional gzip compression
func (d *Daemon) writeResponse(w http.ResponseWriter, r *http.Request, data []byte, startTime time.Time, cached bool) {
	// Check if client accepts gzip
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(data) > 1024 {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gz.Write(data)
	} else {
		w.Write(data)
	}
}

func (d *Daemon) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	stats, err := d.cache.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (d *Daemon) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement cache clear
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cleared",
	})
}

func (d *Daemon) pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apiproxy", "daemon.pid")
}

func (d *Daemon) writePIDFile() error {
	pidPath := d.pidFilePath()
	dir := filepath.Dir(pidPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

func (d *Daemon) removePIDFile() {
	os.Remove(d.pidFilePath())
}
