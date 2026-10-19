package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

// withConfigHome points configuration resolution at a temporary home.
func withConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	testutil.SetHome(t, home)
	for _, key := range []string{
		"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_API_TOKEN",
		"BRIDGE_ERP_BASE", "BRIDGE_AUTH_TYPE", "BRIDGE_API_KEY", "BRIDGE_AUTH_HEADER",
		"BRIDGE_TOKEN", "BRIDGE_USERNAME", "BRIDGE_PASSWORD",
	} {
		t.Setenv(key, "")
	}
	return home
}

func writeRawConfig(t *testing.T, home string) string {
	t.Helper()
	directory := filepath.Join(home, ".bridgectl")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	path := filepath.Join(directory, "config.yaml")
	// #nosec G304 G703 -- the path is built from a test-owned temporary home.
	if err := os.WriteFile(path, []byte(twoContextConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const twoContextConfig = `current-context: local
contexts:
  local:
    server: http://localhost:8082
    mcp-server: http://localhost:8080
    api-token: ${BRIDGE_API_TOKEN}
  staging:
    server: https://staging.internal
    mcp-server: https://staging.internal
    api-token: stored-secret-value
custom-field: keep-me
`

func TestSetCurrentContextChangesOnlyTheDefaultAndKeepsPlaceholders(t *testing.T) {
	home := withConfigHome(t)
	path := writeRawConfig(t, home)

	if err := SetCurrentContext("staging"); err != nil {
		t.Fatalf("SetCurrentContext: %v", err)
	}

	data, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary home
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "current-context: staging") {
		t.Fatalf("saved default was not updated:\n%s", content)
	}
	if !strings.Contains(content, "${BRIDGE_API_TOKEN}") {
		t.Fatalf("environment placeholder was expanded:\n%s", content)
	}
	if !strings.Contains(content, "stored-secret-value") {
		t.Fatalf("stored value was lost:\n%s", content)
	}
	if !strings.Contains(content, "custom-field: keep-me") {
		t.Fatalf("unknown field was dropped:\n%s", content)
	}
	if strings.Contains(content, "https://staging.internal") == false {
		t.Fatalf("endpoint settings were lost:\n%s", content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Fatalf("config mode = %o, want 600", perm)
	}
}

func TestSetCurrentContextLeavesNoTemporaryFiles(t *testing.T) {
	home := withConfigHome(t)
	writeRawConfig(t, home)

	if err := SetCurrentContext("staging"); err != nil {
		t.Fatalf("SetCurrentContext: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".bridgectl"))
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") {
			t.Fatalf("temporary file survived: %s", entry.Name())
		}
	}
}

func TestSetCurrentContextRejectsUnknownContext(t *testing.T) {
	home := withConfigHome(t)
	path := writeRawConfig(t, home)

	if err := SetCurrentContext("missing"); err == nil {
		t.Fatal("an unknown context was accepted")
	}
	before, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary home
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(before), "current-context: local") {
		t.Fatalf("configuration changed after a rejected update:\n%s", before)
	}
}

func TestSetCurrentContextRejectsInvalidName(t *testing.T) {
	withConfigHome(t)
	if err := SetCurrentContext("bad name!"); err == nil {
		t.Fatal("an invalid context name was accepted")
	}
}

func TestSetCurrentContextRefusesExternalChange(t *testing.T) {
	home := withConfigHome(t)
	path := writeRawConfig(t, home)

	beforeReplace = func() {
		beforeReplace = nil
		if err := os.WriteFile(path, []byte(twoContextConfig+"added-by-another-process: true\n"), 0o600); err != nil {
			t.Fatalf("simulate external change: %v", err)
		}
	}
	t.Cleanup(func() { beforeReplace = nil })

	err := SetCurrentContext("staging")
	if !errors.Is(err, ErrConfigChanged) {
		t.Fatalf("error = %v, want ErrConfigChanged", err)
	}
	data, readErr := os.ReadFile(path) // #nosec G304 -- test-owned temporary home
	if readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}
	if !strings.Contains(string(data), "added-by-another-process") {
		t.Fatalf("the external change was overwritten:\n%s", data)
	}
	if !strings.Contains(string(data), "current-context: local") {
		t.Fatalf("the update was applied anyway:\n%s", data)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".bridgectl"))
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".config-") {
			t.Fatalf("temporary file survived a refused update: %s", entry.Name())
		}
	}
}

func TestSetCurrentContextRefusesSymlink(t *testing.T) {
	home := withConfigHome(t)
	directory := filepath.Join(home, ".bridgectl")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	target := filepath.Join(directory, "target.yaml")
	if err := os.WriteFile(target, []byte(twoContextConfig), 0o600); err != nil {
		t.Fatalf("write target config: %v", err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	if err := os.Symlink(target, configPath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	err := SetCurrentContext("staging")
	if err == nil {
		t.Fatal("a configuration symlink was accepted")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v, want a symlink refusal", err)
	}

	// The symlink and its target must both survive untouched.
	info, statErr := os.Lstat(configPath)
	if statErr != nil {
		t.Fatalf("lstat config: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the configuration symlink was replaced by a regular file")
	}
	content, readErr := os.ReadFile(target) // #nosec G304 -- test-owned temporary home
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "current-context: local") {
		t.Fatalf("the symlink target was modified:\n%s", content)
	}
}

func TestSetCurrentContextCreatesDefaultsWhenNoFileExists(t *testing.T) {
	home := withConfigHome(t)
	t.Setenv("BRIDGE_SERVER", "http://environment-only:1234")

	if err := SetCurrentContext(DefaultContextName); err != nil {
		t.Fatalf("SetCurrentContext: %v", err)
	}
	path := filepath.Join(home, ".bridgectl", "config.yaml")
	data, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary home
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "current-context: local") {
		t.Fatalf("created config has no saved default:\n%s", content)
	}
	if strings.Contains(content, "environment-only") {
		t.Fatalf("an environment override was persisted:\n%s", content)
	}

	// The created file must resolve through the normal loader.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after create: %v", err)
	}
	if loaded.CurrentContext != DefaultContextName {
		t.Fatalf("loaded default = %q", loaded.CurrentContext)
	}

	if err := SetCurrentContext("staging"); err == nil {
		t.Fatal("a context that does not exist in the created file was accepted")
	}
}

func TestSetCurrentContextWithoutFileRejectsUnknownContext(t *testing.T) {
	withConfigHome(t)
	if err := SetCurrentContext("staging"); err == nil {
		t.Fatal("an unknown context was accepted without a configuration file")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".bridgectl", "config.yaml")); err == nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatal("a configuration file was created for a rejected update")
	}
}
