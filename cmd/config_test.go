package cmd

import (
	"testing"

	"github.com/afterdarksys/apiproxyd/pkg/config"
)

func TestRedactConfigDoesNotExposeOrMutateSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.APIKey = "apx_live_secret"
	cfg.Cache.PostgresDSN = "postgres://user:password@example/db"
	cfg.Security.MetricsAuthToken = "metrics-secret"
	cfg.Plugins.Plugins = []config.PluginEntry{{
		Config: map[string]interface{}{
			"api_key": "plugin-secret",
			"safe":    "visible",
		},
	}}

	got := redactConfig(cfg)
	if got.APIKey != "[REDACTED]" ||
		got.Cache.PostgresDSN != "[REDACTED]" ||
		got.Security.MetricsAuthToken != "[REDACTED]" {
		t.Fatal("expected top-level secrets to be redacted")
	}
	if got.Plugins.Plugins[0].Config["api_key"] != "[REDACTED]" {
		t.Fatal("expected plugin secret to be redacted")
	}
	if cfg.APIKey != "apx_live_secret" {
		t.Fatal("redaction mutated the live config")
	}
}
