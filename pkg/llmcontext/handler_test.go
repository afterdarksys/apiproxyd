package llmcontext

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerSessionEventPacketFlow(t *testing.T) {
	store := newTestStore(t)
	handler := NewHandler(store, 1024*1024)

	sessionBody := strings.NewReader(`{
		"provider":"openai",
		"working_dir":"/tmp/apiproxyd",
		"git_remote":"https://github.com/afterdarksys/apiproxyd",
		"git_branch":"main"
	}`)
	sessionResp := httptest.NewRecorder()
	handler.ServeHTTP(sessionResp, httptest.NewRequest(http.MethodPost, "/llm/sessions", sessionBody))
	if sessionResp.Code != http.StatusOK {
		t.Fatalf("expected session status 200, got %d: %s", sessionResp.Code, sessionResp.Body.String())
	}

	var session Session
	if err := json.NewDecoder(sessionResp.Body).Decode(&session); err != nil {
		t.Fatalf("failed to decode session: %v", err)
	}
	if session.ID == "" {
		t.Fatal("expected session id")
	}

	eventBody := bytes.NewBufferString(`{
		"session_id":"` + session.ID + `",
		"kind":"decision",
		"source":"architecture",
		"content":"Use /llm endpoints for context packets before provider forwarding."
	}`)
	eventResp := httptest.NewRecorder()
	handler.ServeHTTP(eventResp, httptest.NewRequest(http.MethodPost, "/llm/events", eventBody))
	if eventResp.Code != http.StatusCreated {
		t.Fatalf("expected event status 201, got %d: %s", eventResp.Code, eventResp.Body.String())
	}

	packetBody := bytes.NewBufferString(`{
		"session_id":"` + session.ID + `",
		"question":"What endpoint should build provider context?",
		"max_bytes":800
	}`)
	packetResp := httptest.NewRecorder()
	handler.ServeHTTP(packetResp, httptest.NewRequest(http.MethodPost, "/llm/packet", packetBody))
	if packetResp.Code != http.StatusOK {
		t.Fatalf("expected packet status 200, got %d: %s", packetResp.Code, packetResp.Body.String())
	}

	var packet Packet
	if err := json.NewDecoder(packetResp.Body).Decode(&packet); err != nil {
		t.Fatalf("failed to decode packet: %v", err)
	}
	if !strings.Contains(packet.Content, "/llm endpoints") {
		t.Fatalf("expected packet to include stored context, got %q", packet.Content)
	}
}

func TestHandlerCacheLookupMissAndStoreHit(t *testing.T) {
	store := newTestStore(t)
	handler := NewHandler(store, 1024*1024)

	lookup := `{"provider":"openai","model":"gpt-test","request":{"messages":[{"role":"user","content":"hi"}]}}`
	missResp := httptest.NewRecorder()
	handler.ServeHTTP(missResp, httptest.NewRequest(http.MethodPost, "/llm/cache/lookup", strings.NewReader(lookup)))
	if missResp.Code != http.StatusOK {
		t.Fatalf("expected lookup status 200, got %d: %s", missResp.Code, missResp.Body.String())
	}
	if !strings.Contains(missResp.Body.String(), `"hit":false`) {
		t.Fatalf("expected cache miss response, got %s", missResp.Body.String())
	}

	storeBody := `{
		"provider":"openai",
		"model":"gpt-test",
		"request":{"messages":[{"role":"user","content":"hi"}]},
		"response":{"choices":[{"message":{"content":"hello"}}]},
		"ttl":60
	}`
	storeResp := httptest.NewRecorder()
	handler.ServeHTTP(storeResp, httptest.NewRequest(http.MethodPost, "/llm/cache/store", strings.NewReader(storeBody)))
	if storeResp.Code != http.StatusCreated {
		t.Fatalf("expected store status 201, got %d: %s", storeResp.Code, storeResp.Body.String())
	}

	hitResp := httptest.NewRecorder()
	handler.ServeHTTP(hitResp, httptest.NewRequest(http.MethodPost, "/llm/cache/lookup", strings.NewReader(lookup)))
	if hitResp.Code != http.StatusOK {
		t.Fatalf("expected hit lookup status 200, got %d: %s", hitResp.Code, hitResp.Body.String())
	}
	if !strings.Contains(hitResp.Body.String(), `"hit":true`) {
		t.Fatalf("expected cache hit response, got %s", hitResp.Body.String())
	}
	if !strings.Contains(hitResp.Body.String(), `"hello"`) {
		t.Fatalf("expected cached response body, got %s", hitResp.Body.String())
	}
}
