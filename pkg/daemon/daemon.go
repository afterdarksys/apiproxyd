package daemon

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/client"
	"github.com/afterdarksys/apiproxyd/pkg/cluster"
	"github.com/afterdarksys/apiproxyd/pkg/config"
	"github.com/afterdarksys/apiproxyd/pkg/llmcontext"
	"github.com/afterdarksys/apiproxyd/pkg/metrics"
	"github.com/afterdarksys/apiproxyd/pkg/middleware"
	"github.com/afterdarksys/apiproxyd/pkg/plugin"
	"github.com/afterdarksys/apiproxyd/pkg/queue"
)

type Daemon struct {
	host           string
	port           int
	server         *http.Server
	cache          cache.Cache
	client         *client.Client
	cfg            *config.Config
	pluginManager  *plugin.Manager
	metrics        *metrics.PrometheusMetrics
	rateLimiter    *middleware.RateLimiter
	ssrfProtection *middleware.SSRFProtection
	scheduler      *Scheduler
	llmContext     *llmcontext.Store
	gzipPool       sync.Pool
	singleFlight   *client.SingleFlight

	// Phase 4 Extensions
	taskQueue   queue.TaskQueue
	clusterNode *cluster.Server
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
	if d.host == "" {
		d.host = cfg.Server.Host
	}
	if d.port == 0 {
		d.port = cfg.Server.Port
	}
	if !isLoopbackHost(d.host) && !cfg.Security.AllowRemote {
		return fmt.Errorf(
			"refusing non-loopback bind to %q; set security.allow_remote=true only behind an authenticating network boundary",
			d.host,
		)
	}

	// Initialize cache with advanced options
	cachePath := cfg.Cache.Path
	if cfg.Cache.Backend == "postgres" {
		cachePath = cfg.Cache.PostgresDSN
	}

	cacheOpts := &cache.CacheOptions{
		Backend:            cfg.Cache.Backend,
		Path:               cachePath,
		TTL:                time.Duration(cfg.Cache.TTL) * time.Second,
		MemoryCacheEnabled: cfg.Cache.MemoryCacheEnabled,
		MemoryCacheSize:    cfg.Cache.MemoryCacheSize,
		MaxOpenConns:       cfg.Cache.MaxOpenConns,
		MaxIdleConns:       cfg.Cache.MaxIdleConns,
		ConnMaxLifetime:    time.Duration(cfg.Cache.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime:    time.Duration(cfg.Cache.ConnMaxIdleTime) * time.Second,
		StaleIfError:       cfg.Cache.StaleIfError,
		StaleTTL:           time.Duration(cfg.Cache.StaleTTL) * time.Second,
	}

	cacheStore, err := cache.NewWithOptions(cacheOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	d.cache = cacheStore

	// Start background cache cleanup scheduler
	if cfg.Cache.CleanupInterval > 0 {
		d.scheduler = NewScheduler(d.cache, time.Duration(cfg.Cache.CleanupInterval)*time.Second)
		ctx := context.Background()
		d.scheduler.Start(ctx)
		fmt.Printf("Started cache cleanup scheduler (interval: %ds)\n", cfg.Cache.CleanupInterval)
	}

	// Load API Keys from api_keys.json
	authValue := cfg.APIKey
	authType := "APIKey"

	if apiKeys, err := config.LoadAPIKeys(); err == nil {
		if val, typ := apiKeys.GetPrimaryAuth(); val != "" {
			authValue = val
			authType = typ
		}
	}

	// Initialize client with advanced configuration
	if authValue != "" {
		clientCfg := &client.ClientConfig{
			RequestTimeout:          time.Duration(cfg.Client.RequestTimeout) * time.Second,
			DialTimeout:             time.Duration(cfg.Client.DialTimeout) * time.Second,
			KeepAlive:               time.Duration(cfg.Client.KeepAlive) * time.Second,
			MaxIdleConns:            cfg.Client.MaxIdleConns,
			MaxIdleConnsPerHost:     cfg.Client.MaxIdleConnsPerHost,
			MaxConnsPerHost:         cfg.Client.MaxConnsPerHost,
			IdleConnTimeout:         time.Duration(cfg.Client.IdleConnTimeout) * time.Second,
			TLSHandshakeTimeout:     10 * time.Second,
			ExpectContinueTimeout:   1 * time.Second,
			ResponseHeaderTimeout:   10 * time.Second,
			CircuitBreakerEnabled:   cfg.Client.CircuitBreakerEnabled,
			CircuitBreakerThreshold: cfg.Client.CircuitBreakerThreshold,
			CircuitBreakerTimeout:   time.Duration(cfg.Client.CircuitBreakerTimeout) * time.Second,
			CircuitBreakerHalfOpen:  cfg.Client.CircuitBreakerHalfOpen,
			DeduplicationEnabled:    cfg.Client.DeduplicationEnabled,
		}
		d.client = client.NewWithAuth(authValue, authType, clientCfg)
		d.client.BaseURL = cfg.EntryPoint
	}

	// Initialize request deduplication
	if cfg.Client.DeduplicationEnabled {
		d.singleFlight = client.NewSingleFlight()
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

	// Initialize rate limiter
	if cfg.Security.RateLimitEnabled {
		d.rateLimiter = middleware.NewRateLimiter(
			cfg.Security.RateLimitPerIP,
			cfg.Security.RateLimitPerKey,
			cfg.Security.RateLimitBurst,
		)
		fmt.Printf("Rate limiting enabled: %d req/min per IP, %d req/min per key\n",
			cfg.Security.RateLimitPerIP, cfg.Security.RateLimitPerKey)
	}

	// Initialize SSRF protection
	if cfg.Security.SSRFProtectionEnabled {
		d.ssrfProtection = middleware.NewSSRFProtection(
			cfg.Security.AllowedUpstreamHosts,
			cfg.Security.BlockPrivateIPs,
		)
		fmt.Println("SSRF protection enabled")
	}

	// Initialize Task Queue
	if cfg.Queue.Enabled {
		switch cfg.Queue.Backend {
		case "river":
			if cfg.Cache.PostgresDSN != "" {
				q, err := queue.NewRiverQueue(context.Background(), cfg.Cache.PostgresDSN)
				if err != nil {
					return fmt.Errorf("failed to init river queue: %w", err)
				}
				d.taskQueue = q
			} else {
				fmt.Println("Warning: River queue requested but no postgres_dsn provided")
			}
		case "asynq":
			q, err := queue.NewAsynqQueue(cfg.Queue.RedisAddr)
			if err != nil {
				return fmt.Errorf("failed to init asynq queue: %w", err)
			}
			d.taskQueue = q
		}

		if d.taskQueue != nil {
			if err := d.taskQueue.Start(context.Background()); err != nil {
				return fmt.Errorf("failed to start task queue: %w", err)
			}
			fmt.Printf("Task queue started using backend: %s\n", cfg.Queue.Backend)
		}
	}

	// Initialize gRPC Cluster
	if cfg.Cluster.Enabled {
		d.clusterNode = cluster.NewServer(cfg.Cluster.NodeID, d.cache)
		if err := d.clusterNode.Start(cfg.Cluster.Port); err != nil {
			return fmt.Errorf("failed to start cluster node: %w", err)
		}
		fmt.Printf("Cluster node started on port: %d (ID: %s)\n", cfg.Cluster.Port, cfg.Cluster.NodeID)

		// In a full implementation, the daemon would maintain active connections
		// to the listed `cfg.Cluster.Peers` to broadcast CacheInvalidations.
	}

	if cfg.LLMContext.Enabled {
		store, err := llmcontext.NewSQLiteStore(cfg.LLMContext.Path)
		if err != nil {
			return fmt.Errorf("failed to initialize llm context store: %w", err)
		}
		d.llmContext = store
		fmt.Printf("LLM context store enabled: %s\n", cfg.LLMContext.Path)
	}

	// Create HTTP server with middleware chain
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/api/", d.handleProxy)
	mux.HandleFunc("/cache/stats", d.handleCacheStats)
	mux.HandleFunc("/cache/clear", d.handleCacheClear)
	mux.HandleFunc("/metrics", d.handleMetrics)
	if d.llmContext != nil {
		mux.Handle("/llm/", llmcontext.NewHandler(
			d.llmContext,
			cfg.LLMContext.MaxRequestBytes,
			cfg.LLMContext.DefaultPacketBytes,
		))
	}

	// Build middleware chain
	handler := http.Handler(mux)

	// Add recovery middleware (outermost - catches all panics)
	handler = middleware.RecoveryMiddleware(handler)

	// Add security headers
	handler = middleware.SecureHeaders(handler)

	// Add rate limiting
	if d.rateLimiter != nil {
		handler = d.rateLimiter.Middleware(handler)
	}

	// Add request body size limiting
	if cfg.Security.MaxRequestBodySize > 0 {
		handler = middleware.BodySizeLimiter(cfg.Security.MaxRequestBodySize)(handler)
	}

	// Add input sanitization
	handler = middleware.InputSanitizer(handler)

	d.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", d.host, d.port),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
		// Set max header size to prevent memory exhaustion
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Configure TLS if enabled
	if cfg.Server.TLSEnabled {
		if cfg.Server.TLSCertFile == "" || cfg.Server.TLSKeyFile == "" {
			return fmt.Errorf("TLS enabled but cert/key files not specified")
		}

		d.server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			},
			PreferServerCipherSuites: true,
		}

		// A non-nil empty TLSNextProto map disables automatic HTTP/2.
		if !cfg.Server.EnableHTTP2 {
			d.server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
		}
	}

	listener, err := net.Listen("tcp", d.server.Addr)
	if err != nil {
		d.cleanup()
		return fmt.Errorf("failed to listen on %s: %w", d.server.Addr, err)
	}

	// Write PID file with secure permissions
	if err := d.writePIDFile(); err != nil {
		_ = listener.Close()
		d.cleanup()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		protocol := "http"
		if cfg.Server.TLSEnabled {
			protocol = "https"
		}
		fmt.Printf("Daemon started on %s://%s:%d\n", protocol, d.host, d.port)
		fmt.Printf("   Features enabled:\n")
		if cfg.Cache.MemoryCacheEnabled {
			fmt.Printf("   - In-memory cache (L1): %d entries\n", cfg.Cache.MemoryCacheSize)
		}
		if cfg.Security.RateLimitEnabled {
			fmt.Printf("   - Rate limiting: %d req/min per IP\n", cfg.Security.RateLimitPerIP)
		}
		if cfg.Client.CircuitBreakerEnabled {
			fmt.Printf("   - Circuit breaker: threshold=%d\n", cfg.Client.CircuitBreakerThreshold)
		}
		if cfg.Client.DeduplicationEnabled {
			fmt.Printf("   - Request deduplication\n")
		}
		if cfg.Server.TLSEnabled {
			fmt.Printf("   - TLS/HTTPS\n")
			if cfg.Server.EnableHTTP2 {
				fmt.Printf("   - HTTP/2\n")
			}
		}

		var err error
		if cfg.Server.TLSEnabled {
			err = d.server.ServeTLS(listener, cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			err = d.server.Serve(listener)
		}
		serverErr <- err
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
	case err := <-serverErr:
		signal.Stop(sigChan)
		d.removePIDFile()
		d.cleanup()
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server stopped unexpectedly: %w", err)
		}
		return nil
	}
	signal.Stop(sigChan)
	fmt.Println("\nShutting down daemon...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	d.cleanup()
	d.removePIDFile()

	fmt.Println("Daemon stopped")
	return nil
}

func (d *Daemon) cleanup() {
	if d.scheduler != nil {
		d.scheduler.Stop()
	}
	if d.rateLimiter != nil {
		d.rateLimiter.Close()
	}
	if d.pluginManager != nil {
		_ = d.pluginManager.Shutdown()
	}
	if d.taskQueue != nil {
		_ = d.taskQueue.Stop()
	}
	if d.clusterNode != nil {
		d.clusterNode.Stop()
	}
	if d.llmContext != nil {
		_ = d.llmContext.Close()
	}
	if d.cache != nil {
		_ = d.cache.Close()
	}
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
	fmt.Println("Daemon stopped")
	return nil
}

func (d *Daemon) Status() error {
	if cfg, err := config.Load(); err == nil {
		if d.host == "" {
			d.host = cfg.Server.Host
		}
		if d.port == 0 {
			d.port = cfg.Server.Port
		}
	}

	pidPath := d.pidFilePath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon is not running")
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
		fmt.Println("Daemon is not running")
		return nil
	}

	// Check if process is actually running
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("Daemon is not running (stale PID file)")
		os.Remove(pidPath)
		return nil
	}

	fmt.Printf("Daemon is running (PID: %d)\n", pid)

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

	// Check database connectivity
	healthy := true
	dbStatus := "ok"

	// Try a simple cache operation to verify DB is accessible
	if _, err := d.cache.Stats(); err != nil {
		healthy = false
		dbStatus = fmt.Sprintf("error: %v", err)
	}

	// Add component health checks
	components := make(map[string]interface{})
	if d.client != nil {
		components["upstream_client"] = "ok"
		if d.client.GetCircuitBreakerStats()["state"] == "open" {
			components["upstream_client"] = "circuit_open"
			healthy = false
		}
	}
	if d.rateLimiter != nil {
		components["rate_limiter"] = "ok"
	}

	status := "ok"
	statusCode := http.StatusOK
	if !healthy {
		status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     status,
		"version":    "0.3.0",
		"database":   dbStatus,
		"components": components,
	})
}

func (d *Daemon) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	ctx := r.Context()

	// Extract endpoint path (remove /api prefix)
	endpoint := strings.TrimPrefix(r.URL.Path, "/api")

	// Append query string with semantic deduplication if configured
	if d.cfg.Cache.SemanticDeduplication && len(r.URL.Query()) > 0 {
		// url.Values.Encode() sorts keys alphabetically
		endpoint = endpoint + "?" + r.URL.Query().Encode()
	} else if r.URL.RawQuery != "" {
		endpoint = endpoint + "?" + r.URL.RawQuery
	}

	// Check if endpoint is whitelisted
	if !d.cfg.IsEndpointWhitelisted(endpoint) {
		http.Error(w, fmt.Sprintf("Endpoint not whitelisted: %s", endpoint), http.StatusForbidden)
		return
	}

	// Read body with size limit
	var body []byte
	var err error
	if d.cfg.Security.MaxRequestBodySize > 0 {
		limitedReader := middleware.LimitReader(r.Body, d.cfg.Security.MaxRequestBodySize)
		body, err = io.ReadAll(limitedReader)
		if err != nil {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
	} else {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
	}
	r.Body.Close()

	// Create plugin request
	pluginReq := plugin.FromHTTPRequest(r, body)
	pluginReq.Endpoint = endpoint
	// Only plugins may assign a custom cache key; never trust this header from a client.
	delete(pluginReq.Headers, "X-Custom-Cache-Key")

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
		if !d.cfg.IsEndpointWhitelisted(endpoint) {
			http.Error(w, "Plugin-routed endpoint is not whitelisted", http.StatusForbidden)
			return
		}
	}

	// Generate cache key or use custom key provided by plugin
	var cacheKey string
	if customKey, ok := pluginReq.Headers["X-Custom-Cache-Key"]; ok && customKey != "" {
		cacheKey = customKey
	} else {
		cacheKey = cache.GenerateKey(pluginReq.Method, endpoint, string(body))
	}

	// Check if this is an offline endpoint
	isOffline := d.cfg.IsEndpointOffline(endpoint)
	cacheable := cache.IsCacheableMethod(pluginReq.Method)

	// Try cache first
	if cached, err := d.cache.Get(cacheKey); cacheable && err == nil {
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
		if isOffline && cacheable {
			w.Header().Set("X-Offline", "true")
		}
		for k, v := range pluginResp.Headers {
			w.Header().Set(k, v)
		}
		d.writeResponse(w, r, pluginResp.Body, startTime, true)
		d.metrics.RecordRequest(r.Method, http.StatusOK, time.Since(startTime), true, int64(len(pluginResp.Body)))
		return
	}

	// Check authentication for online requests
	if d.client == nil {
		if isOffline {
			if stale, staleErr := d.cache.GetStale(cacheKey); staleErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "STALE")
				w.Header().Set("X-Offline", "true")
				d.writeResponse(w, r, stale, startTime, true)
				d.metrics.RecordRequest(r.Method, http.StatusOK, time.Since(startTime), true, int64(len(stale)))
				return
			}
		}
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Validate upstream URL if SSRF protection is enabled
	if d.ssrfProtection != nil {
		upstreamURL := d.client.BaseURL + endpoint
		if err := d.ssrfProtection.ValidateURL(upstreamURL); err != nil {
			http.Error(w, "Invalid upstream URL", http.StatusForbidden)
			d.metrics.RecordRequest(r.Method, http.StatusForbidden, time.Since(startTime), false, 0)
			return
		}
	}

	// Make request to API with deduplication
	headers := make(map[string]string)
	for k, v := range pluginReq.Headers {
		headers[k] = v
	}

	var resp []byte
	if d.singleFlight != nil {
		// Use request deduplication
		reqKey := fmt.Sprintf("%s:%s:%s", pluginReq.Method, endpoint, string(body))
		resp, err = d.singleFlight.Do(reqKey, func() ([]byte, error) {
			return d.client.Request(pluginReq.Method, endpoint, bytes.NewReader(body), headers)
		})
	} else {
		resp, err = d.client.Request(pluginReq.Method, endpoint, bytes.NewReader(body), headers)
	}

	if err != nil {
		// Try to serve stale cache if configured
		if cacheable && d.cfg.Cache.StaleIfError {
			if stale, staleErr := d.cache.GetStale(cacheKey); staleErr == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "STALE")
				w.Header().Set("X-Cache-Warning", "110 - \"Response is Stale\"")
				d.writeResponse(w, r, stale, startTime, true)
				d.metrics.RecordRequest(r.Method, http.StatusOK, time.Since(startTime), true, int64(len(stale)))
				return
			}
		}

		// Return safe error message (don't leak internal details)
		http.Error(w, "Upstream service unavailable", http.StatusBadGateway)
		d.metrics.RecordRequest(r.Method, http.StatusBadGateway, time.Since(startTime), false, 0)
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
	if cacheable {
		if err := d.cache.Set(cacheKey, pluginResp.Body); err != nil {
			w.Header().Set("X-Cache-Store", "ERROR")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheable {
		w.Header().Set("X-Cache", "MISS")
	} else {
		w.Header().Set("X-Cache", "BYPASS")
	}
	for k, v := range pluginResp.Headers {
		w.Header().Set(k, v)
	}
	if pluginResp.StatusCode > 0 && pluginResp.StatusCode != http.StatusOK {
		w.WriteHeader(pluginResp.StatusCode)
	}
	d.writeResponse(w, r, pluginResp.Body, startTime, false)
	statusCode := pluginResp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	d.metrics.RecordRequest(r.Method, statusCode, time.Since(startTime), false, int64(len(pluginResp.Body)))
}

// writeResponse writes response with optional gzip compression
func (d *Daemon) writeResponse(w http.ResponseWriter, r *http.Request, data []byte, startTime time.Time, cached bool) {
	// Check response size limit
	if d.cfg.Security.MaxResponseBodySize > 0 && int64(len(data)) > d.cfg.Security.MaxResponseBodySize {
		http.Error(w, "Response too large", http.StatusInternalServerError)
		return
	}

	// Check if client accepts gzip and response is large enough to benefit
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(data) > 1024 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")

		// Use sync.Pool for gzip writers to reduce allocations
		gzObj := d.gzipPool.Get()
		var gz *gzip.Writer
		if gzObj == nil {
			gz, _ = gzip.NewWriterLevel(w, gzip.DefaultCompression)
		} else {
			gz = gzObj.(*gzip.Writer)
			gz.Reset(w)
		}
		defer func() {
			gz.Close()
			d.gzipPool.Put(gz)
		}()

		gz.Write(data)
	} else {
		w.Write(data)
	}
}

func (d *Daemon) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := d.cache.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (d *Daemon) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clearer, ok := d.cache.(interface{ Clear() error })
	if !ok {
		http.Error(w, "Cache backend does not support clearing", http.StatusNotImplemented)
		return
	}
	if err := clearer.Clear(); err != nil {
		http.Error(w, "Failed to clear cache", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cleared",
	})
}

// handleMetrics serves Prometheus metrics with optional authentication
func (d *Daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Check authentication if enabled
	if d.cfg.Security.MetricsAuthEnabled {
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		// Remove "Bearer " prefix if present
		token = strings.TrimPrefix(token, "Bearer ")

		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(token), []byte(d.cfg.Security.MetricsAuthToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Serve metrics
	d.metrics.ServeHTTP(w, r)
}

func (d *Daemon) pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apiproxy", "daemon.pid")
}

func (d *Daemon) writePIDFile() error {
	pidPath := d.pidFilePath()
	dir := filepath.Dir(pidPath)

	// Create directory with restrictive permissions
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	pid := os.Getpid()
	// Write PID file with secure permissions (0600 = owner read/write only)
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0600)
}

func (d *Daemon) removePIDFile() {
	os.Remove(d.pidFilePath())
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
