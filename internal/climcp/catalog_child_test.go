package climcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// cliBuildOnce compiles the real bridgectl binary once per test run so the
// catalogue can be exercised through real child processes instead of mocks.
var (
	cliBuildOnce sync.Once
	cliBuildPath string
	cliBuildErr  error
)

func realCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real child process coverage in short mode")
	}
	cliBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "climcp-cli-")
		if err != nil {
			cliBuildErr = err
			return
		}
		root, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			cliBuildErr = err
			return
		}
		binary := filepath.Join(dir, "bridgectl")
		// #nosec G204 -- fixed argument vector; the output path comes from a
		// private temporary directory created by this test.
		build := exec.Command("go", "build", "-o", binary, "./tools/bridgectl")
		build.Dir = root
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			cliBuildErr = fmt.Errorf("build bridgectl: %w", err)
			return
		}
		cliBuildPath = binary
	})
	if cliBuildErr != nil {
		t.Fatalf("cannot build the bridgectl binary: %v", cliBuildErr)
	}
	return cliBuildPath
}

// deploymentFixture stands in for one ERPBridge deployment and records every
// request so a test can prove reads never write.
type deploymentFixture struct {
	server   *httptest.Server
	tools    string
	plugins  string
	bindings string

	mu           sync.Mutex
	requests     []string
	authHeaders  []string
	probeSuccess bool
	// blockReads delays read endpoints so transport cancellation can be tested.
	blockReads time.Duration
	// blockWrites delays write endpoints for the same purpose.
	blockWrites time.Duration
}

func newDeploymentFixture(t *testing.T) *deploymentFixture {
	t.Helper()
	fixture := &deploymentFixture{
		probeSuccess: true,
		tools: encodeFor(t, []any{map[string]any{
			"apiVersion": "erpbridge.io/v1",
			"kind":       kindTool,
			"metadata": map[string]any{
				"name": "list_items", "version": "1.0.0", "module": "erp",
				"status": "ready", "isActive": true, "isServing": true,
			},
			"spec": map[string]any{
				"description": map[string]any{"short": "List items"},
				"execution": map[string]any{
					"type": "http", "method": "GET",
					"endpoint": "https://erp.internal:8443/api/items?apikey=canary-query", // #nosec G101 -- canary query value
				},
				"security": map[string]any{
					"authType": "api-key", "authHeader": "X-API-Key",
					"credentialRef": "ERP_PRIMARY_KEY", "credentialSource": "env", // #nosec G101 -- logical reference
				},
				"inputSchema": map[string]any{
					"type":     "object",
					"required": []any{"limit"},
					"properties": map[string]any{
						"limit":      map[string]any{"type": "integer", "description": "Maximum rows"},
						"password":   map[string]any{"type": "string", "default": "canary-default"},
						"extraToken": map[string]any{"type": "string", "description": "declared, not a value"},
					},
				},
			},
		}}),
		plugins: encodeFor(t, []any{map[string]any{
			"apiVersion": "erpbridge.io/v1",
			"kind":       kindPlugin,
			"metadata":   map[string]any{"name": "redact", "version": "2.0.0", "type": "api", "isActive": true},
			"spec": map[string]any{
				"endpoint": "https://plugins.internal:9443/v1/process?token=canary-query",            // #nosec G101 -- canary query value
				"auth":     map[string]any{"type": "bearer", "credentialRef": "PLUGIN_REDACT_TOKEN"}, // #nosec G101 -- logical reference
			},
		}}),
		bindings: encodeFor(t, []any{map[string]any{
			"apiVersion": "erpbridge.io/v1",
			"kind":       kindBinding,
			"metadata":   map[string]any{"name": "orders-redaction", "isActive": true},
			"spec": map[string]any{
				"pluginRef":     map[string]any{"name": "redact", "version": "2.0.0"},
				"toolRef":       map[string]any{"name": "list_items", "version": "1.0.0"},
				"phase":         "after_response",
				"priority":      10,
				"failurePolicy": "fail",
				// #nosec G101 -- a canary configuration value.
				"config": map[string]any{"apiKey": "canary-config-value"},
			},
		}}),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		fixture.requests = append(fixture.requests, r.Method+" "+r.URL.Path)
		fixture.authHeaders = append(fixture.authHeaders, r.Header.Get("Authorization"))
		delay := fixture.blockReads
		if r.Method != http.MethodGet {
			delay = fixture.blockWrites
		}
		fixture.mu.Unlock()
		if delay > 0 {
			// Drain the body first, so the server observes a client disconnect
			// while the handler is still waiting.
			_, _ = io.Copy(io.Discard, r.Body)
			// Stop as soon as the caller disconnects, so a cancelled child does
			// not hold the fixture open for the whole delay.
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/erpbridge.io/v1/tools", "/apis/erpbridge.io/v1/plugins", "/apis/erpbridge.io/v1/pluginbindings":
			// A read returns the stored inventory; a write accepts the resource.
			if r.Method != http.MethodGet {
				_, _ = io.WriteString(w, `{}`)
				return
			}
			switch r.URL.Path {
			case "/apis/erpbridge.io/v1/tools":
				_, _ = io.WriteString(w, fixture.tools)
			case "/apis/erpbridge.io/v1/plugins":
				_, _ = io.WriteString(w, fixture.plugins)
			default:
				_, _ = io.WriteString(w, fixture.bindings)
			}
		case "/api/auth/tokens":
			// #nosec G101 -- a canary value used to prove it is never returned.
			_, _ = io.WriteString(w, `[{"id":"t1","name":"agent","scopes":["mcp"],"roles":["viewer"],"createdAt":"2026-01-02T03:04:05Z","token":"canary-token-value","secretHash":"canary-hash"}]`)
		case "/api/auth/tokens/t1":
			_, _ = io.WriteString(w, `{}`)
		case "/api/cache/stats":
			_, _ = io.WriteString(w, `{"stats":{"exactKeys":7,"redisMemory":"2MB"}}`)
		case "/api/logs/recent":
			_, _ = io.WriteString(w, `[{"level":"info","tool_name":"list_items","message":"canary-log-message"},{"level":"info","tool_name":"list_items"},{"level":"error","message":"canary-log-message"}]`)
		case "/api/apis/test":
			fixture.mu.Lock()
			success := fixture.probeSuccess
			fixture.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"status":200,"contentType":"application/json","latency":5,"success":%t}`, success)
		case "/api/cache/flush":
			_, _ = io.WriteString(w, `{"deleted":3,"status":"ok"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"RESOURCE_NOT_FOUND"}`)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *deploymentFixture) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	methods := make([]string, 0, len(f.requests))
	for _, entry := range f.requests {
		method, _, _ := strings.Cut(entry, " ")
		methods = append(methods, method)
	}
	return methods
}

// realSession returns a session whose child is the real bridgectl binary,
// configured against the fixture through a temporary home.
func realSession(t *testing.T, fixture *deploymentFixture) *Server {
	return realSessionWithGroups(t, fixture, nil)
}

// realSessionWithGroups builds a real-child session with the given write groups.
// A second endpoint adds an unused "staging" saved context so a context change
// can be proven not to retarget the session.
func realSessionWithGroups(t *testing.T, fixture *deploymentFixture, groups []WriteGroup, stagingEndpoint ...string) *Server {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{
		"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_API_TOKEN",
		"BRIDGE_ERP_BASE", "BRIDGE_AUTH_TYPE", "BRIDGE_API_KEY", "BRIDGE_AUTH_HEADER",
		"BRIDGE_TOKEN", "BRIDGE_USERNAME", "BRIDGE_PASSWORD", ChildSnapshotEnv,
	} {
		t.Setenv(key, "")
	}
	endpoints := map[string]string{"test": fixture.server.URL}
	if len(stagingEndpoint) > 0 && stagingEndpoint[0] != "" {
		endpoints["staging"] = stagingEndpoint[0]
	}
	writeContextConfigWith(t, home, endpoints)

	snapshot := Snapshot{
		ContextName: "test",
		Server:      fixture.server.URL,
		MCPServer:   fixture.server.URL,
	}
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })

	session, err := NewServer(Options{
		Snapshot: snapshot,
		Version:  "test-version",
		Groups:   groups,
		Runner: &Runner{
			Executable: realCLI(t),
			Env:        snapshot.ChildEnvironment(os.Environ()),
			Timeout:    30 * time.Second,
		},
		Stager: stager,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return session
}

func writeContextConfig(t *testing.T, home, endpoint string) {
	t.Helper()
	writeContextConfigWith(t, home, map[string]string{"test": endpoint})
}

// writeContextConfigWith writes one saved context per endpoint. The staging
// context carries an environment placeholder so a later edit can be checked for
// expansion.
func writeContextConfigWith(t *testing.T, home string, endpoints map[string]string) {
	t.Helper()
	dir := filepath.Join(home, ".bridgectl")
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("create config directory: %v", err)
	}
	var builder strings.Builder
	builder.WriteString("current-context: test\ncontexts:\n")
	for _, name := range sortedCopy(keysOf(endpoints)) {
		endpoint := endpoints[name]
		fmt.Fprintf(&builder, "  %s:\n    server: %s\n    mcp-server: %s\n", name, endpoint, endpoint)
		if name != "test" {
			builder.WriteString("    api-token: ${ERPBRIDGE_TEST_TOKEN}\n")
		}
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(builder.String()), 0o600); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("write config: %v", err)
	}
}

// writeContextConfigWithToken writes one saved context that carries a literal
// token, so a session can be checked for sending the pinned credential.
func writeContextConfigWithToken(t *testing.T, home, endpoint, token string) {
	t.Helper()
	dir := filepath.Join(home, ".bridgectl")
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("create config directory: %v", err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	content := fmt.Sprintf("current-context: test\ncontexts:\n  test:\n    server: %s\n    mcp-server: %s\n    api-token: %s\n",
		endpoint, endpoint, token)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("write config: %v", err)
	}
}

func keysOf(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// registerFixtureAPI writes one API into the context-scoped local registry so
// generation can run against the fixture.
func registerFixtureAPI(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".bridgectl", "registries")
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("create registry directory: %v", err)
	}
	content := `{"apis":{"erp":{"id":"api-1","name":"erp","url":"https://erp.internal:8443/api/items",` +
		`"method":"GET","module":"erp","description":"ERP items","status":"active",` +
		`"authType":"api-key","authHeader":"X-API-Key","credentialRef":"ERP_PRIMARY_KEY",` +
		`"credentialSource":"env","createdAt":"2026-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(dir, "test.json"), []byte(content), 0o600); err != nil { // #nosec G304 G703 -- test-owned temporary home
		t.Fatalf("write registry: %v", err)
	}
}

// fingerprintFiles hashes the local state that a read must never change.
func fingerprintFiles(t *testing.T, home string) string {
	t.Helper()
	digest := sha256.New()
	for _, relative := range []string{".bridgectl/config.yaml", ".bridgectl/registries/test.json"} {
		path := filepath.Join(home, relative)
		data, err := os.ReadFile(path) // #nosec G304 G703 -- test-owned temporary home
		if err != nil {
			_, _ = digest.Write([]byte(relative + ":absent"))
			continue
		}
		_, _ = digest.Write([]byte(relative))
		_, _ = digest.Write(data)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// decodeData re-decodes projected data into its typed form. The dispatch
// helper round-trips through JSON, so data arrives as generic maps.
func decodeData[T any](t *testing.T, data any) T {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal projected data: %v", err)
	}
	var typed T
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("decode projected data: %v", err)
	}
	return typed
}

func TestRealChildReadFamilies(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := realSession(t, fixture)
	home := os.Getenv("HOME")

	cases := []struct {
		tool     string
		args     map[string]any
		canaries []string
		check    func(*testing.T, any)
	}{
		{
			tool:     "bridgectl_context_list",
			canaries: []string{fixture.server.URL, "http://"},
			check: func(t *testing.T, data any) {
				list := decodeData[ContextListData](t, data)
				if len(list.Items) != 1 || list.Items[0].Name != "test" || !list.Items[0].SelectedForSession {
					t.Fatalf("context projection = %+v", list)
				}
			},
		},
		{
			tool: "bridgectl_api_list",
			check: func(t *testing.T, data any) {
				list := decodeData[APIListData](t, data)
				if len(list.Items) != 0 {
					t.Fatalf("api projection = %+v, want an empty registry", list)
				}
			},
		},
		{
			tool:     "bridgectl_tool_get",
			canaries: []string{"canary-query", "canary-default", "/api/items"},
			check: func(t *testing.T, data any) {
				list := decodeData[ResourceData](t, data)
				if !list.Redacted || len(list.Items) != 1 {
					t.Fatalf("tool projection = %+v", list)
				}
				item := list.Items[0]
				if item.Name != "list_items" || item.Version != "1.0.0" || item.Method != "GET" {
					t.Fatalf("tool metadata = %+v", item)
				}
				if item.EndpointLabel != "erp.internal:8443" {
					t.Fatalf("endpointLabel = %q", item.EndpointLabel)
				}
				if _, present := item.InputProperties["password"]; !present {
					t.Fatal("declared password property was dropped")
				}
			},
		},
		{
			tool:     "bridgectl_tool_get",
			args:     map[string]any{"name": "list_items"},
			canaries: []string{"canary-query", "/api/items"},
			check: func(t *testing.T, data any) {
				resource := decodeData[ResourceData](t, data)
				if resource.Item == nil || resource.Item.Name != "list_items" {
					t.Fatalf("named tool projection = %+v", resource)
				}
			},
		},
		{
			tool:     "bridgectl_plugin_get",
			canaries: []string{"canary-query", "/v1/process"},
			check: func(t *testing.T, data any) {
				list := decodeData[ResourceData](t, data)
				if len(list.Items) != 1 || list.Items[0].Name != "redact" || list.Items[0].Version != "2.0.0" {
					t.Fatalf("plugin projection = %+v", list)
				}
			},
		},
		{
			tool:     "bridgectl_plugin_get",
			args:     map[string]any{"name": "redact", "version": "2.0.0"},
			canaries: []string{"canary-query"},
			check: func(t *testing.T, data any) {
				resource := decodeData[ResourceData](t, data)
				if resource.Item == nil || resource.Item.Name != "redact" {
					t.Fatalf("named plugin projection = %+v", resource)
				}
			},
		},
		{
			tool: "bridgectl_binding_get",
			canaries: []string{
				"canary-config-value",
			},
			check: func(t *testing.T, data any) {
				list := decodeData[ResourceData](t, data)
				if len(list.Items) != 1 {
					t.Fatalf("binding projection = %+v", list)
				}
				item := list.Items[0]
				if item.Name != "orders-redaction" || item.Phase != "after_response" || item.FailurePolicy != "fail" {
					t.Fatalf("binding metadata = %+v", item)
				}
				if item.PluginRef == nil || item.PluginRef.Name != "redact" || item.ToolRef == nil || item.ToolRef.Name != "list_items" {
					t.Fatalf("binding references = %+v", item)
				}
			},
		},
		{
			tool:     "bridgectl_token_list",
			canaries: []string{"canary-token-value", "canary-hash", "secretHash"},
			check: func(t *testing.T, data any) {
				list := decodeData[TokenListData](t, data)
				if len(list.Items) != 1 || list.Items[0].ID != "t1" || list.Items[0].Scopes[0] != "mcp" {
					t.Fatalf("token projection = %+v", list)
				}
			},
		},
		{
			tool: "bridgectl_cache_stats",
			check: func(t *testing.T, data any) {
				stats := decodeData[CacheStatsData](t, data)
				if stats.ExactKeys != 7 || stats.RedisMemory != "2MB" {
					t.Fatalf("cache projection = %+v", stats)
				}
			},
		},
		{
			tool:     "bridgectl_log_stats",
			canaries: []string{"canary-log-message"},
			check: func(t *testing.T, data any) {
				stats := decodeData[LogStatsData](t, data)
				if stats.LevelCounts["info"] != 2 || stats.LevelCounts["error"] != 1 {
					t.Fatalf("level counts = %v", stats.LevelCounts)
				}
				if stats.ToolCounts["list_items"] != 2 {
					t.Fatalf("tool counts = %v", stats.ToolCounts)
				}
			},
		},
	}

	before := fingerprintFiles(t, home)
	for _, testCase := range cases {
		t.Run(testCase.tool, func(t *testing.T) {
			result := dispatchTool(t, session, testCase.tool, testCase.args)
			if !result.Success {
				t.Fatalf("%s failed: %s (%s)", testCase.tool, result.Error.Message, result.Error.Suggestion)
			}
			assertNoSecret(t, result.Data, testCase.canaries...)
			if testCase.check != nil {
				testCase.check(t, result.Data)
			}
		})
	}
	if after := fingerprintFiles(t, home); after != before {
		t.Fatal("a read operation changed local registry or configuration state")
	}
	for _, method := range fixture.methods() {
		if method != http.MethodGet {
			t.Fatalf("a read operation issued %s", method)
		}
	}
}

func TestRealChildValidation(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := realSession(t, fixture)

	result := dispatchTool(t, session, "bridgectl_tool_validate", map[string]any{"manifest": validToolManifest()})
	if !result.Success {
		t.Fatalf("valid manifest was rejected: %s", result.Error.Message)
	}
	summary := decodeData[ValidationData](t, result.Data)
	if !summary.Valid || summary.Name != "list_items" {
		t.Fatalf("validation summary = %+v", summary)
	}
	if len(fixture.methods()) != 0 {
		t.Fatalf("local validation issued %v", fixture.methods())
	}

	invalid := validToolManifest()
	delete(invalid["metadata"].(map[string]any), "version")
	result = dispatchTool(t, session, "bridgectl_tool_validate", map[string]any{"manifest": invalid})
	if result.Success {
		t.Fatal("an invalid manifest was reported as valid")
	}
	if result.Error.Code != CodeInvalidArguments {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeInvalidArguments)
	}
	if strings.Contains(result.Error.Message, "metadata.version is missing") {
		t.Fatalf("child validation text leaked: %q", result.Error.Message)
	}
}

func TestRealChildGenerateFromRegisteredAPI(t *testing.T) {
	fixture := newDeploymentFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_API_TOKEN", ChildSnapshotEnv} {
		t.Setenv(key, "")
	}
	writeContextConfig(t, home, fixture.server.URL)
	registerFixtureAPI(t, home)

	snapshot := Snapshot{ContextName: "test", Server: fixture.server.URL, MCPServer: fixture.server.URL}
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	session, err := NewServer(Options{
		Snapshot: snapshot,
		Version:  "test-version",
		Runner: &Runner{
			Executable: realCLI(t),
			Env:        snapshot.ChildEnvironment(os.Environ()),
			Timeout:    30 * time.Second,
		},
		Stager: stager,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	result := dispatchTool(t, session, "bridgectl_tool_generate", map[string]any{"api": "erp"})
	if !result.Success {
		t.Fatalf("generation failed: %s (%s)", result.Error.Message, result.Error.Suggestion)
	}
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal generation data: %v", err)
	}
	if !strings.Contains(string(data), "list_items") && !strings.Contains(string(data), "erp") {
		t.Fatalf("generated manifest does not describe the registered API: %s", data)
	}
	if len(fixture.methods()) != 0 {
		t.Fatalf("generation issued %v", fixture.methods())
	}
}

func TestRealChildUnknownResourceIsNotFound(t *testing.T) {
	// The child reports this with the recognized RESOURCE_NOT_FOUND code, so the
	// adapter never has to read prose to classify it.
	fixture := newDeploymentFixture(t)
	session := realSession(t, fixture)

	result := dispatchTool(t, session, "bridgectl_tool_get", map[string]any{"name": "missing_tool"})
	if result.Success {
		t.Fatal("a missing tool was reported as found")
	}
	if result.Error.Code != CodeNotFound {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeNotFound)
	}
}

func TestRealChildAuthenticationFailureIsMapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"AUTHENTICATION_FAILED","message":"canary-upstream-detail"}`)
	}))
	t.Cleanup(server.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"BRIDGE_CONTEXT", "BRIDGE_SERVER", "BRIDGE_MCP_SERVER", "BRIDGE_API_TOKEN", ChildSnapshotEnv} {
		t.Setenv(key, "")
	}
	writeContextConfig(t, home, server.URL)

	snapshot := Snapshot{ContextName: "test", Server: server.URL, MCPServer: server.URL}
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	session, err := NewServer(Options{
		Snapshot: snapshot,
		Version:  "test-version",
		Runner:   &Runner{Executable: realCLI(t), Env: snapshot.ChildEnvironment(os.Environ()), Timeout: 30 * time.Second},
		Stager:   stager,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	result := dispatchTool(t, session, "bridgectl_token_list", nil)
	if result.Success {
		t.Fatal("an unauthenticated read was reported as successful")
	}
	if result.Error.Code != CodeAuthentication {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeAuthentication)
	}
	if strings.Contains(result.Error.Message, "canary-upstream-detail") {
		t.Fatalf("upstream text leaked into the message: %q", result.Error.Message)
	}
}

func allWriteGroups() []WriteGroup {
	return []WriteGroup{GroupRegistry, GroupResources, GroupDestructive, GroupProbe, GroupContext}
}

func TestRealChildWriteFamilies(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := realSessionWithGroups(t, fixture, allWriteGroups(), fixture.server.URL)
	registerFixtureAPI(t, os.Getenv("HOME"))

	cases := []struct {
		tool       string
		args       map[string]any
		wantTarget string
	}{
		{"bridgectl_api_register", map[string]any{
			"name": "erp-new", "url": "https://erp.internal/api/items", "module": "erp",
			"description": "ERP items", "confirmContext": "test", "confirmTarget": "erp-new",
		}, "erp-new"},
		{"bridgectl_api_set_credential_ref", nil, "erp"},
		{"bridgectl_api_test", nil, "erp"},
		{"bridgectl_tool_apply", nil, "list_items@1.0.0"},
		{"bridgectl_tool_delete", nil, "list_items@1.0.0"},
		{"bridgectl_plugin_apply", nil, "redact@2.0.0"},
		{"bridgectl_plugin_delete", nil, "redact@2.0.0"},
		{"bridgectl_binding_apply", nil, "orders-redaction"},
		{"bridgectl_binding_delete", nil, "orders-redaction"},
		{"bridgectl_token_revoke", nil, "t1"},
		{"bridgectl_cache_flush", map[string]any{
			"selector": "tool", "name": "list_items", "confirmContext": "test", "confirmTarget": "tool:list_items",
		}, "tool:list_items"},
		{"bridgectl_cache_flush", map[string]any{
			"selector": "module", "name": "erp", "confirmContext": "test", "confirmTarget": "module:erp",
		}, "module:erp"},
		{"bridgectl_cache_flush", nil, "all"},
		{"bridgectl_context_set", map[string]any{
			"name": "staging", "confirmContext": "test", "confirmTarget": "staging",
		}, "staging"},
	}

	for _, testCase := range cases {
		args := testCase.args
		if args == nil {
			args = validWriteArgs(t, testCase.tool)
		}
		t.Run(testCase.tool+"_"+testCase.wantTarget, func(t *testing.T) {
			result := dispatchTool(t, session, testCase.tool, args)
			if !result.Success {
				t.Fatalf("%s failed: %s (%s)", testCase.tool, result.Error.Message, result.Error.Suggestion)
			}
			raw, err := json.Marshal(result.Data)
			if err != nil {
				t.Fatalf("marshal write data: %v", err)
			}
			if testCase.tool != "bridgectl_api_test" && !strings.Contains(string(raw), `"verified":false`) {
				t.Fatalf("write result claims verification: %s", raw)
			}
			if testCase.tool == "bridgectl_context_set" {
				if !strings.Contains(string(raw), `"activeContext":"test"`) {
					t.Fatalf("context change reported a different active context: %s", raw)
				}
				return
			}
			if !strings.Contains(string(raw), testCase.wantTarget) {
				t.Fatalf("write data does not name the target %q: %s", testCase.wantTarget, raw)
			}
		})
	}

	// Hard deletion is the same command with an extra confirmed flag.
	hard := validWriteArgs(t, "bridgectl_tool_delete")
	hard["hard"] = true
	result := dispatchTool(t, session, "bridgectl_tool_delete", hard)
	if !result.Success {
		t.Fatalf("hard delete failed: %s", result.Error.Message)
	}

	methods := fixture.methods()
	for _, want := range []string{http.MethodPost, http.MethodDelete, http.MethodGet} {
		found := false
		for _, method := range methods {
			if method == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("the fixture never observed a %s request: %v", want, methods)
		}
	}
}

func TestRealChildProbeFailureIsReported(t *testing.T) {
	fixture := newDeploymentFixture(t)
	fixture.probeSuccess = false
	session := realSessionWithGroups(t, fixture, []WriteGroup{GroupProbe})
	registerFixtureAPI(t, os.Getenv("HOME"))

	result := dispatchTool(t, session, "bridgectl_api_test", validWriteArgs(t, "bridgectl_api_test"))
	if result.Success {
		t.Fatal("a failed probe was reported as success")
	}
	if result.Error.Code != CodeProbeFailed {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeProbeFailed)
	}
	if result.Error.Details == nil || result.Error.Details.HTTPStatus != 200 || result.Error.Details.ProbeSucceeded {
		t.Fatalf("probe details = %+v", result.Error.Details)
	}
}

func TestRealChildDeniedWriteNeverExecutes(t *testing.T) {
	fixture := newDeploymentFixture(t)
	session := realSession(t, fixture)

	for name := range writeToolNames() {
		result := dispatchTool(t, session, name, validWriteArgs(t, name))
		if result.Success {
			t.Fatalf("%s succeeded without its startup group", name)
		}
		if result.Error.Code != CodePermissionDenied {
			t.Fatalf("%s returned %q, want %q", name, result.Error.Code, CodePermissionDenied)
		}
	}
	if methods := fixture.methods(); len(methods) != 0 {
		t.Fatalf("a denied write reached the deployment: %v", methods)
	}
}

func TestRealChildContextSetKeepsTheSessionPinned(t *testing.T) {
	fixture := newDeploymentFixture(t)
	other := newDeploymentFixture(t)
	session := realSessionWithGroups(t, fixture, allWriteGroups(), other.server.URL)
	home := os.Getenv("HOME")
	registerFixtureAPI(t, home)

	// The environment carries a value that must never be persisted.
	t.Setenv("ERPBRIDGE_TEST_TOKEN", "canary-environment-token")

	result := dispatchTool(t, session, "bridgectl_context_set", map[string]any{
		"name": "staging", "confirmContext": "test", "confirmTarget": "staging",
	})
	if !result.Success {
		t.Fatalf("context set failed: %s", result.Error.Message)
	}

	// A later read and write must still use the pinned endpoint.
	read := dispatchTool(t, session, "bridgectl_cache_stats", nil)
	if !read.Success {
		t.Fatalf("read after a context change failed: %s", read.Error.Message)
	}
	write := dispatchTool(t, session, "bridgectl_cache_flush", validWriteArgs(t, "bridgectl_cache_flush"))
	if !write.Success {
		t.Fatalf("write after a context change failed: %s", write.Error.Message)
	}

	if methods := other.methods(); len(methods) != 0 {
		t.Fatalf("the session retargeted the renamed default context: %v", methods)
	}
	if methods := fixture.methods(); len(methods) == 0 {
		t.Fatal("the pinned endpoint received no request")
	}

	// The saved file must keep the placeholder and must not gain an override.
	// #nosec G304 G703 -- test-owned temporary home.
	data, err := os.ReadFile(filepath.Join(home, ".bridgectl", "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "current-context: staging") {
		t.Fatalf("the saved default was not changed:\n%s", content)
	}
	if !strings.Contains(content, "${ERPBRIDGE_TEST_TOKEN}") {
		t.Fatalf("an environment placeholder was expanded:\n%s", content)
	}
	if strings.Contains(content, "canary-environment-token") {
		t.Fatalf("an environment value was persisted:\n%s", content)
	}
	if strings.Contains(content, fixture.server.URL) && strings.Contains(content, other.server.URL) && strings.Count(content, "server:") != 4 {
		t.Fatalf("unexpected endpoint changes:\n%s", content)
	}
}

// newUnauthorizedFixture serves 401 for every request.
func newUnauthorizedFixture(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"AUTHENTICATION_FAILED","message":"canary-upstream-detail"}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// authorizationHeaders returns the Authorization header seen by the fixture.
func (f *deploymentFixture) authorizationHeaders() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.authHeaders...)
}
