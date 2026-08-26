package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Setup temporary home directory
	tmpDir, err := os.MkdirTemp("", "bridgectl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Ensure no env vars interfere
	_ = os.Unsetenv("BRIDGE_CONTEXT")
	_ = os.Unsetenv("BRIDGE_SERVER")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.CurrentContext != DefaultContextName {
		t.Errorf("expected current context '%s', got '%s'", DefaultContextName, cfg.CurrentContext)
	}

	ctx := cfg.ActiveContext()
	if ctx.Server != "http://localhost:8082" {
		t.Errorf("expected server 'http://localhost:8082', got '%s'", ctx.Server)
	}
}

func TestLoad_ReturnsParseErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(os.Getenv("HOME"), ".bridgectl")
	// #nosec G703 -- HOME points to t.TempDir in this test.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- HOME points to t.TempDir in this test.
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("contexts: [not-a-map]"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected malformed YAML error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("parse error must not be reported as missing file: %v", err)
	}
}

func TestLoad_RejectsDeadCurrentContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(os.Getenv("HOME"), ".bridgectl")
	// #nosec G703 -- HOME points to t.TempDir in this test.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := []byte("current-context: dead\ncontexts:\n  local:\n    server: http://localhost:8082\n")
	// #nosec G703 -- HOME points to t.TempDir in this test.
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected dead current context error, got %v", err)
	}
}

func TestConfigEffectiveContextRejectsNilConfigAndMissingContext(t *testing.T) {
	var nilConfig *Config
	if _, err := nilConfig.EffectiveContext(); !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected nil config context error, got %v", err)
	}

	cfg := &Config{CurrentContext: "missing", Contexts: map[string]Context{}}
	if _, err := cfg.EffectiveContext(); !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected missing context error, got %v", err)
	}
}

func TestLoad_RejectsDeadEnvironmentContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BRIDGE_CONTEXT", "missing")
	_, err := Load()
	if !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("expected BRIDGE_CONTEXT selection error, got %v", err)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	// Setup temporary home directory
	tmpDir, err := os.MkdirTemp("", "bridgectl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	_ = os.Setenv("BRIDGE_SERVER", "http://overridden:8082")
	defer func() { _ = os.Unsetenv("BRIDGE_SERVER") }()
	_ = os.Setenv("BRIDGE_API_TOKEN", "env-token")
	defer func() { _ = os.Unsetenv("BRIDGE_API_TOKEN") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	ctx := cfg.ActiveContext()
	if ctx.Server != "http://overridden:8082" {
		t.Errorf("expected server 'http://overridden:8082', got '%s'", ctx.Server)
	}
	if ctx.APIToken != "env-token" {
		t.Errorf("expected API token environment override, got %q", ctx.APIToken)
	}
}
