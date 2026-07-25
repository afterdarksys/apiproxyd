package llmcontext

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("llm context entry not found")

type Store struct {
	db   *sql.DB
	path string
}

type SessionIdentity struct {
	Provider   string            `json:"provider,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	GitRemote  string            `json:"git_remote,omitempty"`
	GitBranch  string            `json:"git_branch,omitempty"`
	GitCommit  string            `json:"git_commit,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Session struct {
	ID         string            `json:"id"`
	Provider   string            `json:"provider,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	GitRemote  string            `json:"git_remote,omitempty"`
	GitBranch  string            `json:"git_branch,omitempty"`
	GitCommit  string            `json:"git_commit,omitempty"`
	RepoKey    string            `json:"repo_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type Event struct {
	ID          int64             `json:"id"`
	SessionID   string            `json:"session_id"`
	Kind        string            `json:"kind"`
	Source      string            `json:"source,omitempty"`
	Content     string            `json:"content"`
	ContentHash string            `json:"content_hash"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type AddEventInput struct {
	SessionID string            `json:"session_id,omitempty"`
	Identity  SessionIdentity   `json:"identity,omitempty"`
	Kind      string            `json:"kind"`
	Source    string            `json:"source,omitempty"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type BuildPacketInput struct {
	SessionID string          `json:"session_id,omitempty"`
	Identity  SessionIdentity `json:"identity,omitempty"`
	Question  string          `json:"question"`
	MaxBytes  int             `json:"max_bytes,omitempty"`
	MaxEvents int             `json:"max_events,omitempty"`
	Kinds     []string        `json:"kinds,omitempty"`
}

type Packet struct {
	SessionID string  `json:"session_id"`
	Question  string  `json:"question"`
	Content   string  `json:"content"`
	Events    []Event `json:"events"`
	Truncated bool    `json:"truncated"`
	Bytes     int     `json:"bytes"`
}

type CachedResponse struct {
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	Key       string            `json:"key"`
	Response  []byte            `json:"response"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

func NewSQLiteStore(path string) (*Store, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".apiproxy", "llm_context.db")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("failed to create llm context directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open llm context database: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping llm context database: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable llm context foreign keys: %w", err)
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize llm context schema: %w", err)
	}

	return &Store{db: db, path: path}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS llm_sessions (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		working_dir TEXT NOT NULL,
		git_remote TEXT NOT NULL,
		git_branch TEXT NOT NULL,
		git_commit TEXT NOT NULL,
		repo_key TEXT NOT NULL,
		metadata TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_llm_sessions_repo_key ON llm_sessions(repo_key);
	CREATE INDEX IF NOT EXISTS idx_llm_sessions_updated_at ON llm_sessions(updated_at);

	CREATE TABLE IF NOT EXISTS llm_context_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		source TEXT NOT NULL,
		content TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		metadata TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(session_id) REFERENCES llm_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_llm_context_events_session ON llm_context_events(session_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_llm_context_events_hash ON llm_context_events(content_hash);
	CREATE INDEX IF NOT EXISTS idx_llm_context_events_kind ON llm_context_events(kind);

	CREATE TABLE IF NOT EXISTS llm_response_cache (
		key TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		response BLOB NOT NULL,
		metadata TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_llm_response_cache_expires ON llm_response_cache(expires_at);
	`

	_, err := db.Exec(schema)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) UpsertSession(identity SessionIdentity) (*Session, error) {
	identity = normalizeIdentity(identity)
	id := SessionID(identity)
	now := time.Now().UTC()
	metadata := marshalStringMap(identity.Metadata)

	_, err := s.db.Exec(`
		INSERT INTO llm_sessions
		(id, provider, working_dir, git_remote, git_branch, git_commit, repo_key, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider = excluded.provider,
			working_dir = excluded.working_dir,
			git_remote = excluded.git_remote,
			git_branch = excluded.git_branch,
			git_commit = excluded.git_commit,
			repo_key = excluded.repo_key,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`, id, identity.Provider, identity.WorkingDir, identity.GitRemote, identity.GitBranch,
		identity.GitCommit, RepoKey(identity), metadata, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert llm session: %w", err)
	}

	return s.GetSession(id)
}

func (s *Store) GetSession(id string) (*Session, error) {
	var session Session
	var metadata string
	err := s.db.QueryRow(`
		SELECT id, provider, working_dir, git_remote, git_branch, git_commit, repo_key, metadata, created_at, updated_at
		FROM llm_sessions
		WHERE id = ?
	`, id).Scan(&session.ID, &session.Provider, &session.WorkingDir, &session.GitRemote,
		&session.GitBranch, &session.GitCommit, &session.RepoKey, &metadata,
		&session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get llm session: %w", err)
	}
	session.Metadata = unmarshalStringMap(metadata)
	return &session, nil
}

func (s *Store) AddEvent(input AddEventInput) (*Event, error) {
	sessionID := input.SessionID
	if sessionID == "" {
		session, err := s.UpsertSession(input.Identity)
		if err != nil {
			return nil, err
		}
		sessionID = session.ID
	}
	if input.Kind == "" {
		input.Kind = "note"
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, fmt.Errorf("content is required")
	}

	now := time.Now().UTC()
	contentHash := hashString(input.Content)
	metadata := marshalStringMap(input.Metadata)

	res, err := s.db.Exec(`
		INSERT INTO llm_context_events
		(session_id, kind, source, content, content_hash, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, input.Kind, input.Source, input.Content, contentHash, metadata, now)
	if err != nil {
		return nil, fmt.Errorf("failed to add llm context event: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get llm context event id: %w", err)
	}

	return &Event{
		ID:          id,
		SessionID:   sessionID,
		Kind:        input.Kind,
		Source:      input.Source,
		Content:     input.Content,
		ContentHash: contentHash,
		Metadata:    input.Metadata,
		CreatedAt:   now,
	}, nil
}

func (s *Store) BuildPacket(input BuildPacketInput) (*Packet, error) {
	sessionID := input.SessionID
	if sessionID == "" {
		session, err := s.UpsertSession(input.Identity)
		if err != nil {
			return nil, err
		}
		sessionID = session.ID
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = 12000
	}
	if input.MaxEvents <= 0 {
		input.MaxEvents = 80
	}

	events, err := s.recentEvents(sessionID, input.MaxEvents, input.Kinds)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(events, func(i, j int) bool {
		a := relevanceScore(input.Question, events[i])
		b := relevanceScore(input.Question, events[j])
		if a == b {
			return events[i].CreatedAt.After(events[j].CreatedAt)
		}
		return a > b
	})

	var b strings.Builder
	b.WriteString("Task Packet\n")
	b.WriteString("Question:\n")
	b.WriteString(strings.TrimSpace(input.Question))
	b.WriteString("\n\nStored Context:\n")

	selected := make([]Event, 0, len(events))
	truncated := false
	for _, event := range events {
		item := formatEvent(event)
		if b.Len()+len(item) > input.MaxBytes {
			truncated = true
			continue
		}
		b.WriteString(item)
		selected = append(selected, event)
	}

	if len(selected) == 0 {
		b.WriteString("- No stored context selected.\n")
	}

	return &Packet{
		SessionID: sessionID,
		Question:  input.Question,
		Content:   b.String(),
		Events:    selected,
		Truncated: truncated,
		Bytes:     b.Len(),
	}, nil
}

func (s *Store) recentEvents(sessionID string, limit int, kinds []string) ([]Event, error) {
	args := []any{sessionID}
	query := `
		SELECT id, session_id, kind, source, content, content_hash, metadata, created_at
		FROM llm_context_events
		WHERE session_id = ?
	`
	if len(kinds) > 0 {
		placeholders := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			placeholders = append(placeholders, "?")
			args = append(args, kind)
		}
		query += " AND kind IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list llm context events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var metadata string
		if err := rows.Scan(&event.ID, &event.SessionID, &event.Kind, &event.Source,
			&event.Content, &event.ContentHash, &metadata, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan llm context event: %w", err)
		}
		event.Metadata = unmarshalStringMap(metadata)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate llm context events: %w", err)
	}

	return events, nil
}

func (s *Store) CacheKey(provider, model string, request []byte) string {
	hash := sha256.New()
	hash.Write([]byte(strings.ToLower(strings.TrimSpace(provider))))
	hash.Write([]byte{0})
	hash.Write([]byte(strings.TrimSpace(model)))
	hash.Write([]byte{0})
	hash.Write(request)
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Store) StoreCachedResponse(provider, model string, request, response []byte, ttl time.Duration, metadata map[string]string) (*CachedResponse, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	key := s.CacheKey(provider, model, request)
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO llm_response_cache
		(key, provider, model, response, metadata, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, key, provider, model, response, marshalStringMap(metadata), now, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to store cached llm response: %w", err)
	}

	return &CachedResponse{
		Provider:  provider,
		Model:     model,
		Key:       key,
		Response:  response,
		Metadata:  metadata,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Store) GetCachedResponse(provider, model string, request []byte) (*CachedResponse, error) {
	key := s.CacheKey(provider, model, request)
	var cached CachedResponse
	var metadata string
	err := s.db.QueryRow(`
		SELECT provider, model, key, response, metadata, created_at, expires_at
		FROM llm_response_cache
		WHERE key = ?
	`, key).Scan(&cached.Provider, &cached.Model, &cached.Key, &cached.Response,
		&metadata, &cached.CreatedAt, &cached.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cached llm response: %w", err)
	}
	if time.Now().UTC().After(cached.ExpiresAt) {
		_, _ = s.db.Exec("DELETE FROM llm_response_cache WHERE key = ?", key)
		return nil, ErrNotFound
	}
	cached.Metadata = unmarshalStringMap(metadata)
	return &cached, nil
}

func RepoKey(identity SessionIdentity) string {
	identity = normalizeIdentity(identity)
	if identity.GitRemote != "" {
		return strings.ToLower(identity.GitRemote)
	}
	if identity.WorkingDir != "" {
		return identity.WorkingDir
	}
	return "unknown"
}

func SessionID(identity SessionIdentity) string {
	identity = normalizeIdentity(identity)
	input := strings.Join([]string{
		strings.ToLower(identity.Provider),
		RepoKey(identity),
		identity.WorkingDir,
		identity.GitBranch,
	}, "\x00")
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])[:32]
}

func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hashString(time.Now().UTC().Format(time.RFC3339Nano))[:32]
	}
	return hex.EncodeToString(b[:])
}

func normalizeIdentity(identity SessionIdentity) SessionIdentity {
	identity.Provider = strings.TrimSpace(identity.Provider)
	identity.WorkingDir = filepath.Clean(strings.TrimSpace(identity.WorkingDir))
	if identity.WorkingDir == "." {
		identity.WorkingDir = ""
	}
	identity.GitRemote = strings.TrimSpace(identity.GitRemote)
	identity.GitBranch = strings.TrimSpace(identity.GitBranch)
	identity.GitCommit = strings.TrimSpace(identity.GitCommit)
	if identity.Metadata == nil {
		identity.Metadata = map[string]string{}
	}
	return identity
}

func relevanceScore(question string, event Event) int {
	terms := termSet(question)
	if len(terms) == 0 {
		return 0
	}

	text := strings.ToLower(event.Kind + " " + event.Source + " " + event.Content)
	score := 0
	for term := range terms {
		if strings.Contains(text, term) {
			score += 2
		}
	}
	if event.Kind == "decision" || event.Kind == "summary" {
		score++
	}
	return score
}

func termSet(text string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-'
	})
	terms := make(map[string]struct{})
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

func formatEvent(event Event) string {
	content := strings.TrimSpace(event.Content)
	if len(content) > 2000 {
		content = content[:2000] + "\n[truncated]"
	}
	source := event.Source
	if source == "" {
		source = "unspecified"
	}
	return fmt.Sprintf("- [%s] %s (%s):\n%s\n", event.Kind, source, event.CreatedAt.Format(time.RFC3339), content)
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func marshalStringMap(value map[string]string) string {
	if len(value) == 0 {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func unmarshalStringMap(value string) map[string]string {
	if value == "" {
		return map[string]string{}
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(value), &result); err != nil || result == nil {
		return map[string]string{}
	}
	return result
}
