package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Cache defines the interface for cache backends
type Cache interface {
	Get(key string) ([]byte, error)
	GetStale(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
	Stats() (*Stats, error)
	Close() error
}

// Stats represents cache statistics
type Stats struct {
	Entries   int64
	SizeBytes int64
	HitRate   float64
	Hits      int64
	Misses    int64
}

// Entry represents a cached item
type Entry struct {
	Key         string
	Value       []byte
	Method      string
	Path        string
	RequestBody string
	StatusCode  int
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Metadata    map[string]string
}

// New creates a new cache backend
func New(backend, path string) (Cache, error) {
	switch backend {
	case "sqlite", "":
		return newSQLite(path, 24*time.Hour, 24*time.Hour)
	case "postgres", "postgresql":
		return newPostgres(path, 24*time.Hour, 24*time.Hour)
	default:
		return nil, fmt.Errorf("unsupported cache backend: %s", backend)
	}
}

// CacheOptions holds configuration for creating a cache
type CacheOptions struct {
	Backend string
	Path    string
	TTL     time.Duration
	// Memory cache options
	MemoryCacheEnabled bool
	MemoryCacheSize    int
	// Connection pool options
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	// Stale cache options
	StaleIfError bool
	StaleTTL     time.Duration
}

// NewWithOptions creates a cache with advanced options
func NewWithOptions(opts *CacheOptions) (Cache, error) {
	if opts == nil {
		return nil, fmt.Errorf("cache options are required")
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	staleTTL := opts.StaleTTL
	if staleTTL < 0 {
		staleTTL = 0
	}

	var dbCache Cache
	var err error

	// Create database cache with connection pooling
	switch opts.Backend {
	case "sqlite", "":
		if opts.MaxOpenConns > 0 {
			dbCache, err = NewSQLiteWithConfig(
				opts.Path,
				ttl,
				staleTTL,
				opts.MaxOpenConns,
				opts.MaxIdleConns,
				opts.ConnMaxLifetime,
				opts.ConnMaxIdleTime,
			)
		} else {
			dbCache, err = newSQLite(opts.Path, ttl, staleTTL)
		}
	case "postgres", "postgresql":
		if opts.MaxOpenConns > 0 {
			dbCache, err = NewPostgresWithConfig(
				opts.Path,
				ttl,
				staleTTL,
				opts.MaxOpenConns,
				opts.MaxIdleConns,
				opts.ConnMaxLifetime,
				opts.ConnMaxIdleTime,
			)
		} else {
			dbCache, err = newPostgres(opts.Path, ttl, staleTTL)
		}
	default:
		return nil, fmt.Errorf("unsupported cache backend: %s", opts.Backend)
	}

	if err != nil {
		return nil, err
	}

	// Wrap with memory cache if enabled
	if opts.MemoryCacheEnabled {
		return NewLayeredCache(dbCache, opts.MemoryCacheSize, ttl, staleTTL), nil
	}

	return dbCache, nil
}

// GenerateKey creates a cache key from request parameters
func GenerateKey(method, path, body string) string {
	hash := sha256.New()
	hash.Write([]byte(method))
	hash.Write([]byte(path))
	hash.Write([]byte(body))
	return hex.EncodeToString(hash.Sum(nil))
}

// IsCacheableMethod reports whether a method is safe to serve from cache by default.
func IsCacheableMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}
