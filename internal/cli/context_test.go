package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

func TestContextListResponse_RenderTable(t *testing.T) {
	resp := &ContextListResponse{
		Items: []ContextItem{
			{Name: "dev", Server: "http://dev", Current: true},
			{Name: "prod", Server: "http://prod", Current: false},
		},
	}
	var buf bytes.Buffer
	err := resp.RenderTable(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("dev")) {
		t.Errorf("expected output to contain 'dev'")
	}
	if !bytes.Contains([]byte(out), []byte("✓")) {
		t.Errorf("expected output to contain '✓'")
	}
}

func TestContextListCmd(t *testing.T) {
	setupTest()
	cfg.Contexts["aaa"] = cfg.Contexts[testContextName]
	cfg.Contexts["zzz"] = cfg.Contexts[testContextName]
	var buf bytes.Buffer
	formatter.Out = &buf

	err := contextListCmd.RunE(contextListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(testContextName)) {
		t.Errorf("expected output to contain 'test'")
	}
	output := buf.String()
	if bytes.Index([]byte(output), []byte("aaa")) > bytes.Index([]byte(output), []byte("test")) ||
		bytes.Index([]byte(output), []byte("test")) > bytes.Index([]byte(output), []byte("zzz")) {
		t.Fatalf("contexts are not sorted: %s", output)
	}
}

func TestContextSetCmd(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	setupTest()

	directory := filepath.Join(home, ".bridgectl")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	configPath := filepath.Join(directory, "config.yaml")
	saved := "current-context: local\ncontexts:\n  local:\n    server: http://localhost:8082\n" +
		"    api-token: ${BRIDGE_API_TOKEN}\n  " + testContextName + ":\n    server: http://test\n"
	if err := os.WriteFile(configPath, []byte(saved), 0o600); err != nil { // #nosec G304 -- test-owned temporary home
		t.Fatalf("write config: %v", err)
	}

	if err := contextSetCmd.RunE(contextSetCmd, []string{testContextName}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	written, err := os.ReadFile(configPath) // #nosec G304 -- test-owned temporary home
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Contains(written, []byte("current-context: "+testContextName)) {
		t.Fatalf("saved default was not updated:\n%s", written)
	}
	if !bytes.Contains(written, []byte("${BRIDGE_API_TOKEN}")) {
		t.Fatalf("environment placeholder was expanded:\n%s", written)
	}

	if err := contextSetCmd.RunE(contextSetCmd, []string{"invalid"}); err == nil {
		t.Fatalf("expected error for invalid context")
	}
	after, err := os.ReadFile(configPath) // #nosec G304 -- test-owned temporary home
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(written, after) {
		t.Fatalf("configuration changed after a rejected update:\n%s", after)
	}
}
