package climcp

import (
	"strings"
	"testing"
)

func TestSnapshotFingerprintCoversEverySelectionField(t *testing.T) {
	base := Snapshot{ContextName: "staging", Server: "http://localhost:8082", MCPServer: "http://localhost:8080", APIToken: "token-a"}

	// Known-good value computed independently as SHA-256 over the
	// length-prefixed fields. A change here must be a deliberate encoding
	// decision, because the fingerprint guards a pinned session.
	const knownGood = "cd3195e224d0e12483f2cec6318e64809fe43bfef2511a0e161daff500159e5f"
	local := Snapshot{ContextName: "local", Server: "http://localhost:8082", MCPServer: "http://localhost:8080"}
	if got := local.Fingerprint(); got != knownGood {
		t.Fatalf("fingerprint encoding changed: got %q, want %q", got, knownGood)
	}

	variants := map[string]Snapshot{
		"context":    {ContextName: "production", Server: base.Server, MCPServer: base.MCPServer, APIToken: base.APIToken},
		"server":     {ContextName: base.ContextName, Server: "http://localhost:9999", MCPServer: base.MCPServer, APIToken: base.APIToken},
		"mcp server": {ContextName: base.ContextName, Server: base.Server, MCPServer: "http://localhost:9999", APIToken: base.APIToken},
		"token":      {ContextName: base.ContextName, Server: base.Server, MCPServer: base.MCPServer, APIToken: "token-b"},
	}
	for name, variant := range variants {
		if variant.Fingerprint() == base.Fingerprint() {
			t.Fatalf("fingerprint ignored the %s field", name)
		}
	}
}

func TestSnapshotFingerprintDistinguishesEmptyFromPresentValues(t *testing.T) {
	withoutToken := Snapshot{ContextName: "staging", Server: "http://localhost:8082", MCPServer: "http://localhost:8080"}
	withToken := Snapshot{ContextName: "staging", Server: "http://localhost:8082", MCPServer: "http://localhost:8080", APIToken: "token-a"}
	if withoutToken.Fingerprint() == withToken.Fingerprint() {
		t.Fatal("an empty token produced the same fingerprint as a configured token")
	}

	withoutEndpoint := Snapshot{ContextName: "staging", MCPServer: "http://localhost:8080"}
	if withoutEndpoint.Fingerprint() == withToken.Fingerprint() {
		t.Fatal("an empty endpoint produced the same fingerprint as a configured endpoint")
	}
}

func TestSnapshotFingerprintIsNotAmbiguousAcrossFieldBoundaries(t *testing.T) {
	left := Snapshot{ContextName: "staging", Server: "ab", MCPServer: "c", APIToken: "token"}.Fingerprint()
	right := Snapshot{ContextName: "staging", Server: "a", MCPServer: "bc", APIToken: "token"}.Fingerprint()
	if left == right {
		t.Fatal("fingerprint is ambiguous when field boundaries move")
	}
}

func TestChildEnvironmentPinsSessionAndRemovesInheritedCopies(t *testing.T) {
	snapshot := Snapshot{ContextName: "staging", Server: "http://localhost:8082", MCPServer: "http://localhost:8080", APIToken: "pinned-token"}
	base := []string{
		"PATH=/usr/bin",
		"BRIDGE_CONTEXT=production",
		"BRIDGE_SERVER=http://elsewhere:1",
		"BRIDGE_MCP_SERVER=http://elsewhere:2",
		"BRIDGE_API_TOKEN=stale-token",
		ChildSnapshotEnv + "=stale-fingerprint",
		"ERP_PRIMARY_KEY=erp-secret",
	}

	env := snapshot.ChildEnvironment(base)

	counts := map[string]int{}
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		counts[key]++
	}
	for _, key := range pinnedChildEnvironment {
		if counts[key] != 1 {
			t.Fatalf("%s appears %d times, want exactly once", key, counts[key])
		}
	}

	want := map[string]string{
		"BRIDGE_CONTEXT":    "BRIDGE_CONTEXT=staging",
		"BRIDGE_SERVER":     "BRIDGE_SERVER=http://localhost:8082",
		"BRIDGE_MCP_SERVER": "BRIDGE_MCP_SERVER=http://localhost:8080",
		"BRIDGE_API_TOKEN":  "BRIDGE_API_TOKEN=pinned-token",
		ChildSnapshotEnv:    ChildSnapshotEnv + "=" + snapshot.Fingerprint(),
	}
	present := map[string]string{}
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, isPinned := want[key]; isPinned {
			present[key] = entry
		}
	}
	for key, value := range want {
		if present[key] != value {
			t.Fatalf("%s = %q, want %q", key, present[key], value)
		}
	}

	if !containsEntry(env, "PATH=/usr/bin") {
		t.Fatal("unrelated inherited environment was dropped")
	}
	if !containsEntry(env, "ERP_PRIMARY_KEY=erp-secret") {
		t.Fatal("required inherited credential source was dropped")
	}
	// The child guard recomputes this value and compares it exactly, so the
	// pinned environment must carry the same fingerprint.
	if present[ChildSnapshotEnv] != ChildSnapshotEnv+"="+snapshot.Fingerprint() {
		t.Fatal("the pinned environment does not carry the session fingerprint")
	}
}

func containsEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
