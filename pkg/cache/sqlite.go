package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteCache struct {
	db   *sql.DB
	path string
	ttl  time.Duration
}

func NewSQLite(path string) (*SQLiteCache, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".apiproxy", "cache.db")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if err := initSQLiteSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteCache{
		db:   db,
		path: path,
		ttl:  24 * time.Hour, // Default 24 hour TTL
	}, nil
}

func initSQLiteSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS cache_entries (
		key TEXT PRIMARY KEY,
		value BLOB NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		request_body TEXT,
		status_code INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_expires_at ON cache_entries(expires_at);
	CREATE INDEX IF NOT EXISTS idx_path ON cache_entries(path);
	CREATE INDEX IF NOT EXISTS idx_created_at ON cache_entries(created_at);
	`

	_, err := db.Exec(schema)
	return err
}

func (c *SQLiteCache) Get(key string) ([]byte, error) {
	var value []byte
	var expiresAt time.Time

	err := c.db.QueryRow(`
		SELECT value, expires_at
		FROM cache_entries
		WHERE key = ?
	`, key).Scan(&value, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache miss")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cache entry: %w", err)
	}

	// Check if expired
	if time.Now().After(expiresAt) {
		c.Delete(key)
		return nil, fmt.Errorf("cache expired")
	}

	return value, nil
}

func (c *SQLiteCache) Set(key string, value []byte) error {
	expiresAt := time.Now().Add(c.ttl)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO cache_entries (key, value, method, path, expires_at)
		VALUES (?, ?, 'UNKNOWN', 'UNKNOWN', ?)
	`, key, value, expiresAt)

	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

func (c *SQLiteCache) SetEntry(entry *Entry) error {
	metadata, _ := json.Marshal(entry.Metadata)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO cache_entries
		(key, value, method, path, request_body, status_code, created_at, expires_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.Key, entry.Value, entry.Method, entry.Path, entry.RequestBody,
		entry.StatusCode, entry.CreatedAt, entry.ExpiresAt, string(metadata))

	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

func (c *SQLiteCache) Delete(key string) error {
	_, err := c.db.Exec("DELETE FROM cache_entries WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}
	return nil
}

func (c *SQLiteCache) Stats() (*Stats, error) {
	var stats Stats

	// Get total entries
	err := c.db.QueryRow("SELECT COUNT(*) FROM cache_entries WHERE expires_at > ?", time.Now()).
		Scan(&stats.Entries)
	if err != nil {
		return nil, fmt.Errorf("failed to get entry count: %w", err)
	}

	// Get total size
	err = c.db.QueryRow("SELECT COALESCE(SUM(LENGTH(value)), 0) FROM cache_entries WHERE expires_at > ?", time.Now()).
		Scan(&stats.SizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get cache size: %w", err)
	}

	// TODO: Track hits/misses for hit rate calculation
	stats.HitRate = 0.0
	stats.Hits = 0
	stats.Misses = 0

	return &stats, nil
}

func (c *SQLiteCache) Close() error {
	return c.db.Close()
}

// CleanupExpired removes all expired entries
func (c *SQLiteCache) CleanupExpired() error {
	_, err := c.db.Exec("DELETE FROM cache_entries WHERE expires_at <= ?", time.Now())
	return err
}
