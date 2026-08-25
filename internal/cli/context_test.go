package cli

import (
	"bytes"
	"testing"
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
	var buf bytes.Buffer
	formatter.Out = &buf

	err := contextListCmd.RunE(contextListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(testContextName)) {
		t.Errorf("expected output to contain 'test'")
	}
}

func TestContextSetCmd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setupTest()

	err := contextSetCmd.RunE(contextSetCmd, []string{testContextName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = contextSetCmd.RunE(contextSetCmd, []string{"invalid"})
	if err == nil {
		t.Fatalf("expected error for invalid context")
	}
}
