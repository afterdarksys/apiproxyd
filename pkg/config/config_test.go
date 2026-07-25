package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPartialConfigAppliesDefaultsAndExpandsHome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"server":{"port":9100},
		"cache":{"path":"~/.apiproxy/test.db"}
	}`), 0600); err != nil {
		t.Fatal(err)
	}

	SetPath(path)
	t.Cleanup(func() { SetPath("") })

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9100 {
		t.Fatalf("expected explicit port 9100, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("expected default host, got %q", cfg.Server.Host)
	}
	if !cfg.Security.SSRFProtectionEnabled {
		t.Fatal("expected omitted security settings to retain secure defaults")
	}
	if cfg.Cache.TTL != 86400 {
		t.Fatalf("expected default cache TTL, got %d", cfg.Cache.TTL)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(home, ".apiproxy", "test.db")
	if cfg.Cache.Path != wantPath {
		t.Fatalf("expected expanded path %q, got %q", wantPath, cfg.Cache.Path)
	}
}

func TestSaveUsesExplicitJSONConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.json")
	SetPath(path)
	t.Cleanup(func() { SetPath("") })

	cfg := Default()
	cfg.Server.Port = 9123
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != 9123 {
		t.Fatalf("expected saved port 9123, got %d", loaded.Server.Port)
	}
}

func TestLoadLegacyFieldsOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{
		"endpoint":"https://legacy.example",
		"daemon_port":9124,
		"cache_backend":"sqlite",
		"cache_path":"~/.apiproxy/legacy.db",
		"cache_ttl":42
	}`), 0600); err != nil {
		t.Fatal(err)
	}

	SetPath(path)
	t.Cleanup(func() { SetPath("") })

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EntryPoint != "https://legacy.example" {
		t.Fatalf("expected legacy endpoint, got %q", cfg.EntryPoint)
	}
	if cfg.Server.Port != 9124 {
		t.Fatalf("expected legacy port, got %d", cfg.Server.Port)
	}
	if cfg.Cache.TTL != 42 {
		t.Fatalf("expected legacy TTL, got %d", cfg.Cache.TTL)
	}
}

func TestSaveCredentialsUpdatesEntryPoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	SetPath(path)
	t.Cleanup(func() { SetPath("") })

	if err := Save(Default()); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials(&Config{
		APIKey:     "apx_test_value",
		EntryPoint: "https://api.example.test",
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "apx_test_value" {
		t.Fatalf("expected saved API key, got %q", cfg.APIKey)
	}
	if cfg.EntryPoint != "https://api.example.test" {
		t.Fatalf("expected saved entry point, got %q", cfg.EntryPoint)
	}
}

func TestLoadAppliesEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	SetPath(path)
	t.Cleanup(func() { SetPath("") })
	t.Setenv("APIPROXY_SERVER_PORT", "9444")
	t.Setenv("APIPROXY_SECURITY_ALLOW_REMOTE", "true")
	t.Setenv("APIPROXY_CACHE_PATH", "~/.apiproxy/from-env.db")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9444 {
		t.Fatalf("expected env port 9444, got %d", cfg.Server.Port)
	}
	if !cfg.Security.AllowRemote {
		t.Fatal("expected remote binding env override")
	}
	if strings.HasPrefix(cfg.Cache.Path, "~") {
		t.Fatalf("expected env path expansion, got %q", cfg.Cache.Path)
	}
}
