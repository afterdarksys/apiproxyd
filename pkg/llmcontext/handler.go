package llmcontext

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	store              *Store
	maxBodySize        int64
	defaultPacketBytes int
}

type cacheLookupRequest struct {
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Request  json.RawMessage `json:"request"`
}

type cacheStoreRequest struct {
	Provider string            `json:"provider"`
	Model    string            `json:"model"`
	Request  json.RawMessage   `json:"request"`
	Response json.RawMessage   `json:"response"`
	TTL      int               `json:"ttl,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func NewHandler(store *Store, maxBodySize int64, defaultPacketBytes ...int) *Handler {
	if maxBodySize <= 0 {
		maxBodySize = 10 * 1024 * 1024
	}
	packetBytes := 12000
	if len(defaultPacketBytes) > 0 && defaultPacketBytes[0] > 0 {
		packetBytes = defaultPacketBytes[0]
	}
	return &Handler{store: store, maxBodySize: maxBodySize, defaultPacketBytes: packetBytes}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimSuffix(r.URL.Path, "/") {
	case "/llm/sessions":
		h.handleSessions(w, r)
	case "/llm/events":
		h.handleEvents(w, r)
	case "/llm/packet":
		h.handlePacket(w, r)
	case "/llm/cache/lookup":
		h.handleCacheLookup(w, r)
	case "/llm/cache/store":
		h.handleCacheStore(w, r)
	default:
		writeJSONError(w, "unknown llm endpoint", http.StatusNotFound)
	}
}

func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input SessionIdentity
	if err := h.decode(r, &input); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := h.store.UpsertSession(input)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, session, http.StatusOK)
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input AddEventInput
	if err := h.decode(r, &input); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	event, err := h.store.AddEvent(input)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, event, http.StatusCreated)
}

func (h *Handler) handlePacket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input BuildPacketInput
	if err := h.decode(r, &input); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.MaxBytes == 0 {
		input.MaxBytes = h.defaultPacketBytes
	}

	packet, err := h.store.BuildPacket(input)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, packet, http.StatusOK)
}

func (h *Handler) handleCacheLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input cacheLookupRequest
	if err := h.decode(r, &input); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Provider == "" || input.Model == "" || len(input.Request) == 0 {
		writeJSONError(w, "provider, model, and request are required", http.StatusBadRequest)
		return
	}

	cached, err := h.store.GetCachedResponse(input.Provider, input.Model, input.Request)
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, map[string]any{
			"hit": false,
			"key": h.store.CacheKey(input.Provider, input.Model, input.Request),
		}, http.StatusOK)
		return
	}
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"hit":        true,
		"key":        cached.Key,
		"provider":   cached.Provider,
		"model":      cached.Model,
		"response":   json.RawMessage(cached.Response),
		"metadata":   cached.Metadata,
		"created_at": cached.CreatedAt,
		"expires_at": cached.ExpiresAt,
	}, http.StatusOK)
}

func (h *Handler) handleCacheStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input cacheStoreRequest
	if err := h.decode(r, &input); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Provider == "" || input.Model == "" || len(input.Request) == 0 || len(input.Response) == 0 {
		writeJSONError(w, "provider, model, request, and response are required", http.StatusBadRequest)
		return
	}

	cached, err := h.store.StoreCachedResponse(
		input.Provider,
		input.Model,
		input.Request,
		input.Response,
		time.Duration(input.TTL)*time.Second,
		input.Metadata,
	)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"key":        cached.Key,
		"provider":   cached.Provider,
		"model":      cached.Model,
		"created_at": cached.CreatedAt,
		"expires_at": cached.ExpiresAt,
	}, http.StatusCreated)
}

func (h *Handler) decode(r *http.Request, target any) error {
	defer r.Body.Close()
	body := http.MaxBytesReader(nil, r.Body, h.maxBodySize)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON document")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	if status == http.StatusMethodNotAllowed {
		w.Header().Set("Allow", "POST")
	}
	if status == http.StatusRequestEntityTooLarge {
		message = "request body too large: " + strconv.Itoa(status)
	}
	writeJSON(w, map[string]string{"error": message}, status)
}
