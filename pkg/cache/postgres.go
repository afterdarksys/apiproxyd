package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type PostgresCache struct {
	db  *sql.DB
	dsn string
	ttl time.Duration
}

func NewPostgres(dsn string) (*PostgresCache, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Configure connection pool for PostgreSQL
	// PostgreSQL handles concurrency well, so we can have more connections
	db.SetMaxOpenConns(25)       // Max concurrent connections
	db.SetMaxIdleConns(5)        // Keep connections warm for reuse
	db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections periodically
	db.SetConnMaxIdleTime(1 * time.Minute) // Close idle connections

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Initialize schema
	if err := initPostgresSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &PostgresCache{
		db:  db,
		dsn: dsn,
		ttl: 24 * time.Hour,
	}, nil
}

// NewPostgresWithConfig creates a Postgres cache with custom connection pool settings
func NewPostgresWithConfig(dsn string, maxOpen, maxIdle int, maxLifetime, maxIdleTime time.Duration) (*PostgresCache, error) {
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Apply custom pool configuration
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)
	db.SetConnMaxIdleTime(maxIdleTime)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Initialize schema
	if err := initPostgresSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &PostgresCache{
		db:  db,
		dsn: dsn,
		ttl: 24 * time.Hour,
	}, nil
}

func initPostgresSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS apiproxy_cache (
		key TEXT PRIMARY KEY,
		value BYTEA NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		request_body TEXT,
		status_code INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		metadata JSONB
	);

	CREATE INDEX IF NOT EXISTS idx_apiproxy_cache_expires_at ON apiproxy_cache(expires_at);
	CREATE INDEX IF NOT EXISTS idx_apiproxy_cache_path ON apiproxy_cache(path);
	CREATE INDEX IF NOT EXISTS idx_apiproxy_cache_created_at ON apiproxy_cache(created_at);
	`

	_, err := db.Exec(schema)
	return err
}

func (c *PostgresCache) Get(key string) ([]byte, error) {
	var value []byte
	var expiresAt time.Time

	err := c.db.QueryRow(`
		SELECT value, expires_at
		FROM apiproxy_cache
		WHERE key = $1
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

func (c *PostgresCache) Set(key string, value []byte) error {
	expiresAt := time.Now().Add(c.ttl)

	_, err := c.db.Exec(`
		INSERT INTO apiproxy_cache (key, value, method, path, expires_at)
		VALUES ($1, $2, 'UNKNOWN', 'UNKNOWN', $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			expires_at = EXCLUDED.expires_at
	`, key, value, expiresAt)

	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

func (c *PostgresCache) SetEntry(entry *Entry) error {
	metadata, _ := json.Marshal(entry.Metadata)

	_, err := c.db.Exec(`
		INSERT INTO apiproxy_cache
		(key, value, method, path, request_body, status_code, created_at, expires_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			method = EXCLUDED.method,
			path = EXCLUDED.path,
			request_body = EXCLUDED.request_body,
			status_code = EXCLUDED.status_code,
			expires_at = EXCLUDED.expires_at,
			metadata = EXCLUDED.metadata
	`, entry.Key, entry.Value, entry.Method, entry.Path, entry.RequestBody,
		entry.StatusCode, entry.CreatedAt, entry.ExpiresAt, metadata)

	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

func (c *PostgresCache) Delete(key string) error {
	_, err := c.db.Exec("DELETE FROM apiproxy_cache WHERE key = $1", key)
	if err != nil {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}
	return nil
}

func (c *PostgresCache) Stats() (*Stats, error) {
	var stats Stats

	// Get total entries
	err := c.db.QueryRow("SELECT COUNT(*) FROM apiproxy_cache WHERE expires_at > $1", time.Now()).
		Scan(&stats.Entries)
	if err != nil {
		return nil, fmt.Errorf("failed to get entry count: %w", err)
	}

	// Get total size
	err = c.db.QueryRow("SELECT COALESCE(SUM(LENGTH(value)), 0) FROM apiproxy_cache WHERE expires_at > $1", time.Now()).
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

func (c *PostgresCache) Close() error {
	return c.db.Close()
}

// CleanupExpired removes all expired entries
func (c *PostgresCache) CleanupExpired() error {
	_, err := c.db.Exec("DELETE FROM apiproxy_cache WHERE expires_at <= $1", time.Now())
	return err
}
