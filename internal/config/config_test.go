package config

import (
	"log/slog"
	"testing"
)

// envMap builds a getenv func backed by a map.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, envMap(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8084 {
		t.Errorf("Port = %d, want 8084", cfg.Port)
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver = %q, want sqlite", cfg.DBDriver)
	}
	if cfg.BaseURL != "http://localhost:8084" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadFlagOverridesDefault(t *testing.T) {
	cfg, err := Load([]string{"--port", "9000", "--base-url", "https://hooks.example"}, envMap(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.BaseURL != "https://hooks.example" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestEnvOverridesFlag(t *testing.T) {
	// House convention: env wins over flag.
	env := envMap(map[string]string{
		"RAPTOR_PORT":     "7000",
		"RAPTOR_BASE_URL": "https://env.example",
	})
	cfg, err := Load([]string{"--port", "9000", "--base-url", "https://flag.example"}, env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 7000 {
		t.Errorf("Port = %d, want 7000 (env overrides flag)", cfg.Port)
	}
	if cfg.BaseURL != "https://env.example" {
		t.Errorf("BaseURL = %q, want env value", cfg.BaseURL)
	}
}

func TestEnvOverridesDefaultWithoutFlag(t *testing.T) {
	env := envMap(map[string]string{"RAPTOR_REQUIRE_AUTH": "true", "RAPTOR_DATA": "/srv/data"})
	cfg, err := Load(nil, env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.RequireAuth {
		t.Error("RequireAuth = false, want true from env")
	}
	if cfg.Data != "/srv/data" {
		t.Errorf("Data = %q", cfg.Data)
	}
}

func TestVersionFlag(t *testing.T) {
	cfg, err := Load([]string{"--version"}, envMap(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ShowVersion {
		t.Error("ShowVersion = false, want true")
	}
}

func TestInvalidValues(t *testing.T) {
	if _, err := Load([]string{"--log-level", "bogus"}, envMap(nil)); err == nil {
		t.Error("expected error for invalid log level")
	}
	if _, err := Load([]string{"--db-driver", "mysql"}, envMap(nil)); err == nil {
		t.Error("expected error for invalid db driver")
	}
	if _, err := Load(nil, envMap(map[string]string{"RAPTOR_PORT": "notanint"})); err == nil {
		t.Error("expected error for invalid RAPTOR_PORT")
	}
}

func TestSlogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for in, want := range cases {
		cfg := Config{LogLevel: in}
		if got := cfg.SlogLevel(); got != want {
			t.Errorf("SlogLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
