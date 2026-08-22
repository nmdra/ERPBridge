package config

import (
	"os"
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
