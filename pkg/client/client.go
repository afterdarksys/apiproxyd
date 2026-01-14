package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Client struct {
	APIKey         string
	BaseURL        string
	HTTPClient     *http.Client
	circuitBreaker *CircuitBreaker
	singleFlight   *SingleFlight
}

type KeyInfo struct {
	Valid        bool   `json:"valid"`
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Tier         string `json:"tier"`
	RateLimit    int    `json:"rate_limit"`
	MonthlyQuota int    `json:"monthly_quota"`
}

// ClientConfig holds configuration for creating an HTTP client
type ClientConfig struct {
	RequestTimeout          time.Duration
	DialTimeout             time.Duration
	KeepAlive               time.Duration
	MaxIdleConns            int
	MaxIdleConnsPerHost     int
	MaxConnsPerHost         int
	IdleConnTimeout         time.Duration
	TLSHandshakeTimeout     time.Duration
	ExpectContinueTimeout   time.Duration
	ResponseHeaderTimeout   time.Duration
	CircuitBreakerEnabled   bool
	CircuitBreakerThreshold int
	CircuitBreakerTimeout   time.Duration
	CircuitBreakerHalfOpen  int
	DeduplicationEnabled    bool
}

// DefaultClientConfig returns sensible defaults for production use
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		RequestTimeout:          30 * time.Second,
		DialTimeout:             10 * time.Second,
		KeepAlive:               30 * time.Second,
		MaxIdleConns:            100,
		MaxIdleConnsPerHost:     10,
		MaxConnsPerHost:         100,
		IdleConnTimeout:         90 * time.Second,
		TLSHandshakeTimeout:     10 * time.Second,
		ExpectContinueTimeout:   1 * time.Second,
		ResponseHeaderTimeout:   10 * time.Second,
		CircuitBreakerEnabled:   true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   60 * time.Second,
		CircuitBreakerHalfOpen:  3,
		DeduplicationEnabled:    true,
	}
}

func New(apiKey string) *Client {
	return NewWithConfig(apiKey, DefaultClientConfig())
}

// NewWithConfig creates a new client with custom configuration
func NewWithConfig(apiKey string, cfg *ClientConfig) *Client {
	// Create custom transport with connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: cfg.KeepAlive,
		}).DialContext,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		// Enable HTTP/2
		ForceAttemptHTTP2: true,
		// TLS configuration
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Prefer modern cipher suites
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			},
		},
	}

	client := &Client{
		APIKey:  apiKey,
		BaseURL: "https://api.apiproxy.app",
		HTTPClient: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: transport,
		},
	}

	// Enable circuit breaker if configured
	if cfg.CircuitBreakerEnabled {
		client.circuitBreaker = NewCircuitBreaker(
			cfg.CircuitBreakerThreshold,
			cfg.CircuitBreakerTimeout,
			cfg.CircuitBreakerHalfOpen,
		)
	}

	// Enable request deduplication if configured
	if cfg.DeduplicationEnabled {
		client.singleFlight = NewSingleFlight()
	}

	return client
}

// ValidateKey validates the API key with the server
func (c *Client) ValidateKey() (*KeyInfo, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/v1/validate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("authentication failed: %s", string(body))
	}

	var info KeyInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// Request makes an API request through the proxy
func (c *Client) Request(method, path string, body io.Reader, headers map[string]string) ([]byte, error) {
	// Use request deduplication if enabled
	if c.singleFlight != nil {
		// Create a unique key for this request
		// Note: This is a simple implementation. For POST/PUT with different bodies,
		// you might want to include a hash of the body in the key
		key := fmt.Sprintf("%s:%s", method, path)

		return c.singleFlight.Do(key, func() ([]byte, error) {
			return c.doRequest(method, path, body, headers)
		})
	}

	return c.doRequest(method, path, body, headers)
}

// doRequest performs the actual HTTP request with circuit breaker protection
func (c *Client) doRequest(method, path string, body io.Reader, headers map[string]string) ([]byte, error) {
	url := c.BaseURL + path

	// Use circuit breaker if enabled
	if c.circuitBreaker != nil {
		var result []byte
		err := c.circuitBreaker.Call(func() error {
			var callErr error
			result, callErr = c.executeRequest(method, url, body, headers)
			return callErr
		})
		return result, err
	}

	return c.executeRequest(method, url, body, headers)
}

// executeRequest performs the raw HTTP request
func (c *Client) executeRequest(method, url string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip") // Enable compression

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (c *Client) GetCircuitBreakerStats() map[string]interface{} {
	if c.circuitBreaker == nil {
		return map[string]interface{}{"enabled": false}
	}
	stats := c.circuitBreaker.Stats()
	stats["enabled"] = true
	return stats
}

// GetSingleFlightStats returns request deduplication statistics
func (c *Client) GetSingleFlightStats() map[string]interface{} {
	if c.singleFlight == nil {
		return map[string]interface{}{"enabled": false}
	}
	stats := c.singleFlight.Stats()
	stats["enabled"] = true
	return stats
}
