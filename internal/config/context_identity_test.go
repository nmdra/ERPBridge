package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func identityConfigHome(t *testing.T, document string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_ERP_BASE", "BRIDGE_API_TOKEN", "BRIDGE_AUTH_TYPE", "BRIDGE_API_KEY", "BRIDGE_AUTH_HEADER", "BRIDGE_TOKEN", "BRIDGE_USERNAME", "BRIDGE_PASSWORD"} {
		t.Setenv(name, "")
	}
	path := filepath.Join(home, ".bridgectl", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContextIdentityLoadCasePair(t *testing.T) {
	document := "current-context: local\ncontexts:\n  local:\n    server: local-server\n  prod:\n    server: lower\n  Prod:\n    server: upper\n"
	path := identityConfigHome(t, document)
	t.Setenv("BRIDGE_CONTEXT", "prod")
	cfg, err := Load()
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "distinct names") {
			t.Fatalf("expected ambiguous-name refusal, got %v", err)
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		ctx, err := cfg.EffectiveContext()
		if err != nil || ctx.Server != "lower" {
			t.Fatalf("case-sensitive environment selection changed: %+v, %v", ctx, err)
		}
		if cfg.Contexts["Prod"].Server != "upper" {
			t.Fatal("case-distinct context changed")
		}
	}
	got, err := os.ReadFile(path) // #nosec G304 -- path is confined to the test fixture home.
	if err != nil || string(got) != document {
		t.Fatalf("Load changed original: %q, %v", got, err)
	}
}

func TestContextIdentitySetCasePair(t *testing.T) {
	document := "current-context: local\ncontexts:\n  local: {}\n  prod: {}\n  Prod: {}\n"
	path := identityConfigHome(t, document)
	err := SetCurrentContext("local")
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "distinct names") {
			t.Fatalf("expected ambiguous-name refusal, got %v", err)
		}
		got, readErr := os.ReadFile(path) // #nosec G304 -- path is confined to the test fixture home.
		if readErr != nil || string(got) != document {
			t.Fatalf("refusal changed original: %q, %v", got, readErr)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatalf("unexpected write artifacts: %v", entries)
	}
}

func TestContextIdentitySelectionRemainsSyntaxOnly(t *testing.T) {
	for _, name := range []string{"CON", "NUL.test", "console", "com10", "prod", "Prod"} {
		t.Run(name, func(t *testing.T) {
			identityConfigHome(t, "current-context: "+name+"\ncontexts:\n  "+name+": {}\n")
			if _, err := Load(); err != nil {
				t.Fatal(err)
			}
			if err := SetCurrentContext(name); err != nil {
				t.Fatal(err)
			}
		})
	}
}
