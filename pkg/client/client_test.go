package client

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testClientForServer(server *httptest.Server) *Client {
	cfg := DefaultClientConfig()
	cfg.CircuitBreakerEnabled = false
	client := NewWithConfig("configured-key", cfg)
	client.BaseURL = server.URL
	return client
}

func TestRequestAutomaticallyDecompressesGzipResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			t.Error("expected Go transport to negotiate gzip")
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(`{"ok":true}`))
		_ = gz.Close()
	}))
	defer server.Close()

	got, err := testClientForServer(server).Request(http.MethodGet, "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("expected decompressed response, got %q", got)
	}
}

func TestRequestDeduplicationIncludesBody(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		_, _ = fmt.Fprintf(w, "%s", body)
	}))
	defer server.Close()

	client := testClientForServer(server)
	start := make(chan struct{})
	results := make(chan string, 2)
	for _, body := range []string{`{"id":1}`, `{"id":2}`} {
		body := body
		go func() {
			<-start
			got, err := client.Request(http.MethodPost, "/same-path", strings.NewReader(body), nil)
			if err != nil {
				results <- "error:" + err.Error()
				return
			}
			results <- string(got)
		}()
	}
	close(start)

	got := map[string]bool{<-results: true, <-results: true}
	if !got[`{"id":1}`] || !got[`{"id":2}`] {
		t.Fatalf("expected distinct responses for distinct bodies, got %#v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected two upstream calls, got %d", calls)
	}
}

func TestConfiguredAuthenticationCannotBeOverridden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "configured-key" {
			t.Errorf("expected configured key, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected caller authorization to be removed, got %q", got)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	_, err := testClientForServer(server).Request(http.MethodGet, "/", nil, map[string]string{
		"X-API-Key":     "attacker-key",
		"Authorization": "Bearer attacker-token",
	})
	if err != nil {
		t.Fatal(err)
	}
}
