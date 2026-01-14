package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel defines the severity of an audit event
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelCritical
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Event represents an audit log event
type Event struct {
	Timestamp  time.Time         `json:"timestamp"`
	Level      string            `json:"level"`
	EventType  string            `json:"event_type"`
	UserID     string            `json:"user_id,omitempty"`
	APIKey     string            `json:"api_key,omitempty"` // masked
	IP         string            `json:"ip,omitempty"`
	Method     string            `json:"method,omitempty"`
	Path       string            `json:"path,omitempty"`
	StatusCode int               `json:"status_code,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Cached     bool              `json:"cached,omitempty"`
	Message    string            `json:"message"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Logger handles audit logging with rotation and structured output
type Logger struct {
	mu            sync.Mutex
	file          *os.File
	path          string
	maxSize       int64 // bytes
	maxAge        time.Duration
	minLevel      LogLevel
	jsonFormat    bool
	console       bool
	buffer        []Event
	bufferSize    int
	flushInterval time.Duration
	done          chan struct{}
}

// Config defines audit logger configuration
type Config struct {
	Enabled       bool          `json:"enabled"`
	Path          string        `json:"path"`
	MaxSizeMB     int           `json:"max_size_mb"`     // file size before rotation
	MaxAgeDays    int           `json:"max_age_days"`    // days to keep logs
	Level         string        `json:"level"`           // minimum log level
	JSONFormat    bool          `json:"json_format"`     // JSON vs plain text
	Console       bool          `json:"console"`         // also log to console
	BufferSize    int           `json:"buffer_size"`     // number of events to buffer
	FlushInterval int           `json:"flush_interval"`  // seconds between flushes
}

// NewLogger creates a new audit logger
func NewLogger(config *Config) (*Logger, error) {
	if !config.Enabled {
		return &Logger{done: make(chan struct{})}, nil
	}

	// Expand home directory in path
	logPath := config.Path
	if logPath[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		logPath = filepath.Join(home, logPath[2:])
	}

	// Ensure directory exists
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	maxSize := int64(config.MaxSizeMB) * 1024 * 1024
	if maxSize == 0 {
		maxSize = 100 * 1024 * 1024 // 100MB default
	}

	maxAge := time.Duration(config.MaxAgeDays) * 24 * time.Hour
	if maxAge == 0 {
		maxAge = 30 * 24 * time.Hour // 30 days default
	}

	minLevel := parseLogLevel(config.Level)

	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = 100
	}

	flushInterval := time.Duration(config.FlushInterval) * time.Second
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	logger := &Logger{
		file:          file,
		path:          logPath,
		maxSize:       maxSize,
		maxAge:        maxAge,
		minLevel:      minLevel,
		jsonFormat:    config.JSONFormat,
		console:       config.Console,
		buffer:        make([]Event, 0, bufferSize),
		bufferSize:    bufferSize,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
	}

	// Start background flusher
	go logger.flusher()

	// Start log rotation checker
	go logger.rotationChecker()

	return logger, nil
}

// Log logs an audit event
func (l *Logger) Log(level LogLevel, eventType, message string, metadata map[string]string) {
	if l.file == nil {
		return // logging disabled
	}

	if level < l.minLevel {
		return // below minimum level
	}

	event := Event{
		Timestamp: time.Now(),
		Level:     level.String(),
		EventType: eventType,
		Message:   message,
		Metadata:  metadata,
	}

	l.mu.Lock()
	l.buffer = append(l.buffer, event)
	shouldFlush := len(l.buffer) >= l.bufferSize
	l.mu.Unlock()

	if shouldFlush {
		l.Flush()
	}
}

// LogRequest logs an HTTP request
func (l *Logger) LogRequest(method, path, ip, apiKey string, statusCode int, duration time.Duration, cached bool) {
	if l.file == nil {
		return
	}

	// Mask API key (show only first/last 4 chars)
	maskedKey := maskAPIKey(apiKey)

	event := Event{
		Timestamp:  time.Now(),
		Level:      LevelInfo.String(),
		EventType:  "http_request",
		APIKey:     maskedKey,
		IP:         ip,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		Duration:   duration,
		Cached:     cached,
		Message:    fmt.Sprintf("%s %s -> %d (%v)", method, path, statusCode, duration),
	}

	l.mu.Lock()
	l.buffer = append(l.buffer, event)
	shouldFlush := len(l.buffer) >= l.bufferSize
	l.mu.Unlock()

	if shouldFlush {
		l.Flush()
	}
}

// LogAuth logs authentication events
func (l *Logger) LogAuth(apiKey, ip string, success bool, reason string) {
	level := LevelInfo
	if !success {
		level = LevelWarn
	}

	message := "Authentication successful"
	if !success {
		message = fmt.Sprintf("Authentication failed: %s", reason)
	}

	metadata := map[string]string{
		"api_key": maskAPIKey(apiKey),
		"ip":      ip,
		"success": fmt.Sprintf("%v", success),
	}

	l.Log(level, "authentication", message, metadata)
}

// LogRateLimit logs rate limit violations
func (l *Logger) LogRateLimit(ip, apiKey string) {
	metadata := map[string]string{
		"ip": ip,
	}
	if apiKey != "" {
		metadata["api_key"] = maskAPIKey(apiKey)
	}

	l.Log(LevelWarn, "rate_limit", "Rate limit exceeded", metadata)
}

// LogError logs error events
func (l *Logger) LogError(context, message string, err error) {
	metadata := map[string]string{
		"context": context,
	}
	if err != nil {
		metadata["error"] = err.Error()
	}

	l.Log(LevelError, "error", message, metadata)
}

// Flush writes buffered events to disk
func (l *Logger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buffer) == 0 || l.file == nil {
		return nil
	}

	for _, event := range l.buffer {
		if err := l.writeEvent(event); err != nil {
			return err
		}
	}

	// Clear buffer
	l.buffer = l.buffer[:0]

	// Sync to disk
	return l.file.Sync()
}

// writeEvent writes a single event to the log file
func (l *Logger) writeEvent(event Event) error {
	var output []byte
	var err error

	if l.jsonFormat {
		output, err = json.Marshal(event)
		if err != nil {
			return err
		}
		output = append(output, '\n')
	} else {
		// Plain text format
		output = []byte(fmt.Sprintf("[%s] %s %s: %s\n",
			event.Timestamp.Format(time.RFC3339),
			event.Level,
			event.EventType,
			event.Message,
		))
	}

	// Write to file
	if _, err := l.file.Write(output); err != nil {
		return err
	}

	// Also write to console if enabled
	if l.console {
		os.Stdout.Write(output)
	}

	return nil
}

// flusher periodically flushes the buffer
func (l *Logger) flusher() {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.Flush()
		case <-l.done:
			l.Flush() // final flush
			return
		}
	}
}

// rotationChecker checks if log rotation is needed
func (l *Logger) rotationChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.checkRotation()
		case <-l.done:
			return
		}
	}
}

// checkRotation rotates the log file if needed
func (l *Logger) checkRotation() {
	if l.file == nil {
		return
	}

	// Check file size
	info, err := l.file.Stat()
	if err != nil {
		return
	}

	if info.Size() >= l.maxSize {
		l.rotate()
	}

	// Clean up old log files
	l.cleanupOldLogs()
}

// rotate rotates the current log file
func (l *Logger) rotate() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Flush any buffered events
	for _, event := range l.buffer {
		l.writeEvent(event)
	}
	l.buffer = l.buffer[:0]

	// Close current file
	l.file.Close()

	// Rename current file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s", l.path, timestamp)
	os.Rename(l.path, rotatedPath)

	// Open new file
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	l.file = file
}

// cleanupOldLogs removes log files older than maxAge
func (l *Logger) cleanupOldLogs() {
	dir := filepath.Dir(l.path)
	pattern := filepath.Base(l.path) + ".*"

	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-l.maxAge)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
	}
}

// Close closes the audit logger
func (l *Logger) Close() error {
	close(l.done)

	if l.file != nil {
		l.Flush()
		return l.file.Close()
	}

	return nil
}

// maskAPIKey masks an API key for logging
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// parseLogLevel parses a log level string
func parseLogLevel(level string) LogLevel {
	switch level {
	case "DEBUG", "debug":
		return LevelDebug
	case "INFO", "info":
		return LevelInfo
	case "WARN", "warn":
		return LevelWarn
	case "ERROR", "error":
		return LevelError
	case "CRITICAL", "critical":
		return LevelCritical
	default:
		return LevelInfo
	}
}

// Helper function to copy io.Reader to Writer with limit
func copyWithLimit(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	return io.CopyN(dst, src, limit)
}
