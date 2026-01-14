package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type KeyInfo struct {
	Valid        bool   `json:"valid"`
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Tier         string `json:"tier"`
	RateLimit    int    `json:"rate_limit"`
	MonthlyQuota int    `json:"monthly_quota"`
}

func New(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: "https://api.apiproxy.app",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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
	url := c.BaseURL + path

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")

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
