package config

import (
	"os"
	"strings"
	"testing"
)

func TestContextIdentityLoadChecksBeforeEnvironmentExpansion(t *testing.T) {
	document := "current-context: local\ncontexts:\n  local: {}\n  prod:\n    api-token: ${IDENTITY_BAD_YAML}\n  Prod: {}\n"
	identityConfigHome(t, document)
	t.Setenv("IDENTITY_BAD_YAML", "[invalid")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "distinct names") {
		t.Fatalf("must refuse ambiguous names before expanding values: %v", err)
	}
	if strings.Contains(err.Error(), "invalid") {
		t.Fatalf("error exposes expanded content: %v", err)
	}
}

func TestContextIdentitySetChecksRawKeys(t *testing.T) {
	document := "current-context: local\ncontexts:\n  local: {}\n  '${IDENTITY_NAME}': {}\n  '${identity_name}': {}\n"
	path := identityConfigHome(t, document)
	t.Setenv("IDENTITY_NAME", "unused")
	if err := SetCurrentContext("local"); err == nil || !strings.Contains(err.Error(), "distinct names") {
		t.Fatalf("must check raw document: %v", err)
	}
	got, err := os.ReadFile(path) // #nosec G304 -- path is confined to the test fixture home.
	if err != nil || string(got) != document {
		t.Fatalf("refusal changed original: %q, %v", got, err)
	}
}
