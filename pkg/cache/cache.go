package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Cache defines the interface for cache backends
type Cache interface {
	Get(key string) ([]byte, error)
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
	Key        string
	Value      []byte
	Method     string
	Path       string
	RequestBody string
	StatusCode int
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Metadata   map[string]string
}

// New creates a new cache backend
func New(backend, path string) (Cache, error) {
	switch backend {
	case "sqlite", "":
		return NewSQLite(path)
	case "postgres", "postgresql":
		return NewPostgres(path)
	default:
		return nil, fmt.Errorf("unsupported cache backend: %s", backend)
	}
}

// GenerateKey creates a cache key from request parameters
func GenerateKey(method, path, body string) string {
	hash := sha256.New()
	hash.Write([]byte(method))
	hash.Write([]byte(path))
	hash.Write([]byte(body))
	return hex.EncodeToString(hash.Sum(nil))
}
