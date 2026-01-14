package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/plugin"
)

//go:embed templates/*
var templates embed.FS

// WebAdminPlugin provides a web-based admin interface for debugging and monitoring
type WebAdminPlugin struct {
	config      map[string]interface{}
	server      *http.Server
	port        int
	requests    []RequestLog
	mu          sync.RWMutex
	maxRequests int
	startTime   time.Time
	stats       *Stats
}

// RequestLog stores information about a proxied request
type RequestLog struct {
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Endpoint     string    `json:"endpoint"`
	StatusCode   int       `json:"status_code"`
	Duration     int64     `json:"duration_ms"`
	Cached       bool      `json:"cached"`
	BodySize     int       `json:"body_size"`
	Headers      map[string]string `json:"headers,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
}

// Stats tracks overall statistics
type Stats struct {
	TotalRequests   int64   `json:"total_requests"`
	CacheHits       int64   `json:"cache_hits"`
	CacheMisses     int64   `json:"cache_misses"`
	TotalBytes      int64   `json:"total_bytes"`
	AvgResponseTime float64 `json:"avg_response_time_ms"`
	Uptime          float64 `json:"uptime_seconds"`
}

// NewPlugin is the required factory function for Go plugins
func NewPlugin() plugin.Plugin {
	return &WebAdminPlugin{
		requests:    make([]RequestLog, 0),
		maxRequests: 1000, // Keep last 1000 requests
		startTime:   time.Now(),
		stats:       &Stats{},
	}
}

func (w *WebAdminPlugin) Name() string {
	return "web_admin"
}

func (w *WebAdminPlugin) Version() string {
	return "1.0.0"
}

func (w *WebAdminPlugin) Init(config map[string]interface{}) error {
	w.config = config

	// Get port from config (default 9003)
	w.port = 9003
	if port, ok := config["port"].(float64); ok {
		w.port = int(port)
	} else if port, ok := config["port"].(int); ok {
		w.port = port
	}

	// Get max requests to keep
	if maxReq, ok := config["max_requests"].(float64); ok {
		w.maxRequests = int(maxReq)
	} else if maxReq, ok := config["max_requests"].(int); ok {
		w.maxRequests = maxReq
	}

	// Start the web server
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleDashboard)
	mux.HandleFunc("/api/stats", w.handleAPIStats)
	mux.HandleFunc("/api/requests", w.handleAPIRequests)
	mux.HandleFunc("/api/requests/clear", w.handleAPIClearRequests)

	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", w.port),
		Handler: mux,
	}

	// Start server in background
	go func() {
		fmt.Printf("[Web Admin] Starting admin interface on http://localhost:%d\n", w.port)
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Web Admin] Server error: %v\n", err)
		}
	}()

	return nil
}

func (w *WebAdminPlugin) OnRequest(ctx context.Context, req *plugin.Request) (*plugin.Request, bool, error) {
	// Store request start time in metadata
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	req.Metadata["start_time"] = time.Now().Format(time.RFC3339Nano)

	return req, true, nil
}

func (w *WebAdminPlugin) OnResponse(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	w.logRequest(req, resp, false)
	return resp, nil
}

func (w *WebAdminPlugin) OnCacheHit(ctx context.Context, req *plugin.Request, resp *plugin.Response) (*plugin.Response, error) {
	w.logRequest(req, resp, true)
	return resp, nil
}

func (w *WebAdminPlugin) logRequest(req *plugin.Request, resp *plugin.Response, cached bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Calculate duration
	var duration int64
	if startTimeStr, ok := req.Metadata["start_time"]; ok {
		if startTime, err := time.Parse(time.RFC3339Nano, startTimeStr); err == nil {
			duration = time.Since(startTime).Milliseconds()
		}
	}

	// Limit response body size for display
	responseBody := string(resp.Body)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000] + "... (truncated)"
	}

	log := RequestLog{
		Timestamp:    time.Now(),
		Method:       req.Method,
		Endpoint:     req.Endpoint,
		StatusCode:   resp.StatusCode,
		Duration:     duration,
		Cached:       cached,
		BodySize:     len(resp.Body),
		Headers:      req.Headers,
		ResponseBody: responseBody,
	}

	// Add to requests log (keep only last N)
	w.requests = append(w.requests, log)
	if len(w.requests) > w.maxRequests {
		w.requests = w.requests[len(w.requests)-w.maxRequests:]
	}

	// Update stats
	w.stats.TotalRequests++
	w.stats.TotalBytes += int64(len(resp.Body))
	if cached {
		w.stats.CacheHits++
	} else {
		w.stats.CacheMisses++
	}

	// Update average response time
	if w.stats.TotalRequests > 0 {
		totalDuration := w.stats.AvgResponseTime * float64(w.stats.TotalRequests-1)
		w.stats.AvgResponseTime = (totalDuration + float64(duration)) / float64(w.stats.TotalRequests)
	}
}

func (w *WebAdminPlugin) Shutdown() error {
	fmt.Println("[Web Admin] Shutting down admin interface")
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return w.server.Shutdown(ctx)
	}
	return nil
}

// HTTP Handlers

func (w *WebAdminPlugin) handleDashboard(wr http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>apiproxyd Admin Interface</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0f0f23;
            color: #e0e0e0;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        h1 {
            color: #00ff88;
            margin-bottom: 10px;
            font-size: 2em;
        }
        .subtitle {
            color: #888;
            margin-bottom: 30px;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .stat-card {
            background: #1a1a2e;
            border: 1px solid #333;
            border-radius: 8px;
            padding: 20px;
        }
        .stat-label {
            color: #888;
            font-size: 0.9em;
            margin-bottom: 5px;
        }
        .stat-value {
            color: #00ff88;
            font-size: 2em;
            font-weight: bold;
        }
        .stat-value.red { color: #ff5555; }
        .stat-value.blue { color: #5599ff; }
        .stat-value.yellow { color: #ffaa00; }
        .section {
            background: #1a1a2e;
            border: 1px solid #333;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
        }
        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
        }
        h2 {
            color: #00ff88;
            font-size: 1.5em;
        }
        button {
            background: #00ff88;
            color: #0f0f23;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
            font-weight: bold;
        }
        button:hover { background: #00dd77; }
        .request-log {
            max-height: 600px;
            overflow-y: auto;
        }
        .request-item {
            background: #0f0f23;
            border: 1px solid #333;
            border-radius: 4px;
            padding: 15px;
            margin-bottom: 10px;
            cursor: pointer;
        }
        .request-item:hover {
            border-color: #00ff88;
        }
        .request-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }
        .method {
            font-weight: bold;
            padding: 2px 8px;
            border-radius: 3px;
            font-size: 0.85em;
        }
        .method.GET { background: #5599ff; color: white; }
        .method.POST { background: #00ff88; color: #0f0f23; }
        .method.PUT { background: #ffaa00; color: #0f0f23; }
        .method.DELETE { background: #ff5555; color: white; }
        .endpoint {
            color: #00ff88;
            font-family: monospace;
            flex-grow: 1;
            margin: 0 15px;
        }
        .badge {
            padding: 2px 8px;
            border-radius: 3px;
            font-size: 0.85em;
            margin-left: 5px;
        }
        .badge.cached { background: #5599ff; color: white; }
        .badge.miss { background: #333; color: #888; }
        .request-meta {
            display: flex;
            gap: 15px;
            font-size: 0.9em;
            color: #888;
        }
        .duration { color: #ffaa00; }
        .status-200 { color: #00ff88; }
        .status-400 { color: #ffaa00; }
        .status-500 { color: #ff5555; }
        .request-details {
            display: none;
            margin-top: 15px;
            padding-top: 15px;
            border-top: 1px solid #333;
        }
        .request-item.expanded .request-details {
            display: block;
        }
        pre {
            background: #0a0a18;
            padding: 10px;
            border-radius: 4px;
            overflow-x: auto;
            font-size: 0.85em;
        }
        .refresh-info {
            color: #888;
            font-size: 0.9em;
            margin-top: 10px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 apiproxyd Admin Interface</h1>
        <p class="subtitle">Debug, monitor, and inspect your API proxy in real-time</p>

        <div class="stats-grid" id="stats">
            <div class="stat-card">
                <div class="stat-label">Total Requests</div>
                <div class="stat-value" id="total-requests">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Cache Hits</div>
                <div class="stat-value blue" id="cache-hits">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Cache Misses</div>
                <div class="stat-value red" id="cache-misses">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Hit Rate</div>
                <div class="stat-value yellow" id="hit-rate">0%</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Avg Response Time</div>
                <div class="stat-value" id="avg-time">0ms</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Uptime</div>
                <div class="stat-value blue" id="uptime">0s</div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>📋 Request Log</h2>
                <button onclick="clearRequests()">Clear Log</button>
            </div>
            <div class="request-log" id="request-log">
                <p style="color: #888;">No requests yet. Make some API calls to see them here.</p>
            </div>
            <p class="refresh-info">Auto-refreshing every 2 seconds</p>
        </div>
    </div>

    <script>
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
        }

        function formatDuration(seconds) {
            if (seconds < 60) return Math.round(seconds) + 's';
            if (seconds < 3600) return Math.round(seconds / 60) + 'm';
            return Math.round(seconds / 3600) + 'h';
        }

        function toggleRequestDetails(element) {
            element.classList.toggle('expanded');
        }

        async function clearRequests() {
            try {
                await fetch('/api/requests/clear', { method: 'POST' });
                updateData();
            } catch (e) {
                console.error('Failed to clear requests:', e);
            }
        }

        async function updateData() {
            try {
                // Fetch stats
                const statsRes = await fetch('/api/stats');
                const stats = await statsRes.json();

                document.getElementById('total-requests').textContent = stats.total_requests;
                document.getElementById('cache-hits').textContent = stats.cache_hits;
                document.getElementById('cache-misses').textContent = stats.cache_misses;

                const hitRate = stats.total_requests > 0
                    ? Math.round((stats.cache_hits / stats.total_requests) * 100)
                    : 0;
                document.getElementById('hit-rate').textContent = hitRate + '%';
                document.getElementById('avg-time').textContent = Math.round(stats.avg_response_time_ms) + 'ms';
                document.getElementById('uptime').textContent = formatDuration(stats.uptime_seconds);

                // Fetch requests
                const reqRes = await fetch('/api/requests');
                const requests = await reqRes.json();

                const logEl = document.getElementById('request-log');
                if (requests.length === 0) {
                    logEl.innerHTML = '<p style="color: #888;">No requests yet. Make some API calls to see them here.</p>';
                } else {
                    logEl.innerHTML = requests.reverse().map(req => {
                        const timestamp = new Date(req.timestamp).toLocaleTimeString();
                        const statusClass = req.status_code < 400 ? 'status-200' :
                                          req.status_code < 500 ? 'status-400' : 'status-500';
                        return \`
                            <div class="request-item" onclick="toggleRequestDetails(this)">
                                <div class="request-header">
                                    <span class="method \${req.method}">\${req.method}</span>
                                    <span class="endpoint">\${req.endpoint}</span>
                                    <span class="badge \${req.cached ? 'cached' : 'miss'}">
                                        \${req.cached ? '⚡ CACHED' : 'MISS'}
                                    </span>
                                </div>
                                <div class="request-meta">
                                    <span>\${timestamp}</span>
                                    <span class="\${statusClass}">Status: \${req.status_code}</span>
                                    <span class="duration">⏱ \${req.duration_ms}ms</span>
                                    <span>📦 \${formatBytes(req.body_size)}</span>
                                </div>
                                <div class="request-details">
                                    <h3 style="color: #00ff88; margin-bottom: 10px;">Response Body</h3>
                                    <pre>\${req.response_body || 'No body'}</pre>
                                </div>
                            </div>
                        \`;
                    }).join('');
                }
            } catch (e) {
                console.error('Failed to update data:', e);
            }
        }

        // Initial load and auto-refresh
        updateData();
        setInterval(updateData, 2000);
    </script>
</body>
</html>`

	wr.Header().Set("Content-Type", "text/html")
	wr.Write([]byte(tmpl))
}

func (w *WebAdminPlugin) handleAPIStats(wr http.ResponseWriter, r *http.Request) {
	w.mu.RLock()
	stats := *w.stats
	stats.Uptime = time.Since(w.startTime).Seconds()
	w.mu.RUnlock()

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(stats)
}

func (w *WebAdminPlugin) handleAPIRequests(wr http.ResponseWriter, r *http.Request) {
	w.mu.RLock()
	requests := make([]RequestLog, len(w.requests))
	copy(requests, w.requests)
	w.mu.RUnlock()

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(requests)
}

func (w *WebAdminPlugin) handleAPIClearRequests(wr http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(wr, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.mu.Lock()
	w.requests = make([]RequestLog, 0)
	w.mu.Unlock()

	wr.Header().Set("Content-Type", "application/json")
	json.NewEncoder(wr).Encode(map[string]string{"status": "cleared"})
}

func main() {
	// Required for Go plugins
	p := NewPlugin()
	data, _ := json.MarshalIndent(map[string]string{
		"name":    p.Name(),
		"version": p.Version(),
	}, "", "  ")
	fmt.Println(string(data))
}
