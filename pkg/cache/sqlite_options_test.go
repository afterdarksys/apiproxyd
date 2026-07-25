package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteCacheHonorsConfiguredTTLAndStaleWindow(t *testing.T) {
	store, err := NewWithOptions(&CacheOptions{
		Backend:  "sqlite",
		Path:     filepath.Join(t.TempDir(), "cache.db"),
		TTL:      37 * time.Second,
		StaleTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sqliteStore, ok := store.(*SQLiteCache)
	if !ok {
		t.Fatalf("expected SQLiteCache, got %T", store)
	}
	if sqliteStore.ttl != 37*time.Second {
		t.Fatalf("expected TTL 37s, got %s", sqliteStore.ttl)
	}

	now := time.Now()
	if err := sqliteStore.SetEntry(&Entry{
		Key:       "within-window",
		Value:     []byte("stale"),
		CreatedAt: now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := sqliteStore.GetStale("within-window"); err != nil || string(got) != "stale" {
		t.Fatalf("expected usable stale entry, got %q, %v", got, err)
	}

	if err := sqliteStore.SetEntry(&Entry{
		Key:       "outside-window",
		Value:     []byte("too-old"),
		CreatedAt: now.Add(-3 * time.Minute),
		ExpiresAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteStore.GetStale("outside-window"); err == nil {
		t.Fatal("expected entry outside stale window to be rejected")
	}
}

func TestSQLiteCacheClearRemovesAllEntries(t *testing.T) {
	store, err := newSQLite(filepath.Join(t.TempDir(), "cache.db"), time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.Set("one", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("two", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 0 {
		t.Fatalf("expected empty cache, got %d entries", stats.Entries)
	}
}

func TestIsCacheableMethod(t *testing.T) {
	for _, method := range []string{"GET", "get", "HEAD"} {
		if !IsCacheableMethod(method) {
			t.Fatalf("expected %s to be cacheable", method)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if IsCacheableMethod(method) {
			t.Fatalf("expected %s not to be cacheable", method)
		}
	}
}
