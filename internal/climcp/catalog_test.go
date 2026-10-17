package climcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// defaultToolNames is the planned default catalogue: inspection, local
// validation, and generation only. It is a literal on purpose so a change to
// the descriptor table cannot silently widen the advertised set.
func defaultToolNames() []string {
	return []string{
		"bridgectl_api_list",
		"bridgectl_binding_get",
		"bridgectl_binding_validate",
		"bridgectl_cache_stats",
		"bridgectl_context_list",
		"bridgectl_log_stats",
		"bridgectl_plugin_get",
		"bridgectl_plugin_validate",
		"bridgectl_token_list",
		"bridgectl_tool_generate",
		"bridgectl_tool_get",
		"bridgectl_tool_validate",
	}
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	slices.Sort(copied)
	return copied
}

func equalStrings(left, right []string) bool { return slices.Equal(left, right) }

func descriptorNamed(t *testing.T, name string) descriptor {
	t.Helper()
	for _, entry := range catalogue {
		if entry.name == name {
			return entry
		}
	}
	t.Fatalf("no descriptor named %q", name)
	return descriptor{}
}

// testInvocationContext returns an invocation context backed by a real private
// staging directory.
func testInvocationContext(t *testing.T) invocationContext {
	t.Helper()
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	return invocationContext{operation: "bridgectl_test", contextName: "test", stage: stager.Stage}
}

// sessionWithoutChild builds a session whose child executable does not exist, so
// reaching execution is observable as an internal error.
func sessionWithoutChild(t *testing.T) *Server {
	t.Helper()
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	session, err := NewServer(Options{
		Snapshot: testSnapshot(),
		Version:  "test-version",
		Stager:   stager,
		Runner:   &Runner{Executable: filepath.Join(t.TempDir(), "no-such-binary")},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return session
}

// dispatchTool calls one advertised tool through the real dispatch pipeline.
func dispatchTool(t *testing.T, session *Server, name string, args map[string]any) Result {
	t.Helper()
	descriptor := descriptorNamed(t, name)
	call, err := session.dispatch(context.Background(), descriptor, args)
	if err != nil {
		t.Fatalf("dispatch %s: %v", name, err)
	}
	return decodeEnvelope(t, call)
}

// decodeEnvelope reads the envelope exactly as a client receives it.
func decodeEnvelope(t *testing.T, call *mcp.CallToolResult) Result {
	t.Helper()
	raw, err := json.Marshal(call.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if result.Success == call.IsError {
		t.Fatalf("isError %t disagrees with success %t", call.IsError, result.Success)
	}
	if len(call.Content) != 1 {
		t.Fatalf("expected one text content item, got %d", len(call.Content))
	}
	text, ok := call.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content item is %T, want mcp.TextContent", call.Content[0])
	}
	if strings.TrimSpace(text.Text) != string(raw) {
		t.Fatalf("text content %q does not match structured content %s", text.Text, raw)
	}
	return result
}

// encodeFor marshals a value and fails on error.
func encodeFor(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// assertNoSecret fails when a marshalled projection contains a canary value.
func assertNoSecret(t *testing.T, data any, canaries ...string) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	for _, canary := range canaries {
		if canary == "" {
			continue
		}
		if strings.Contains(string(raw), canary) {
			t.Fatalf("projection leaked %q: %s", canary, raw)
		}
	}
}

// writeToolNames is the planned write and probe catalogue with its groups. It is
// a literal on purpose so a change to the descriptor table cannot silently widen
// what a startup grant exposes.
func writeToolNames() map[string]WriteGroup {
	return map[string]WriteGroup{
		"bridgectl_api_register":           GroupRegistry,
		"bridgectl_api_set_credential_ref": GroupRegistry,
		"bridgectl_api_test":               GroupProbe,
		"bridgectl_tool_apply":             GroupResources,
		"bridgectl_tool_delete":            GroupDestructive,
		"bridgectl_plugin_apply":           GroupResources,
		"bridgectl_plugin_delete":          GroupDestructive,
		"bridgectl_binding_apply":          GroupResources,
		"bridgectl_binding_delete":         GroupDestructive,
		"bridgectl_token_revoke":           GroupDestructive,
		"bridgectl_cache_flush":            GroupDestructive,
		"bridgectl_context_set":            GroupContext,
	}
}

func TestCatalogueMatchesPlannedSet(t *testing.T) {
	grouped := make(map[WriteGroup][]string)
	advertised := make([]string, 0, len(catalogue))
	for _, entry := range catalogue {
		if entry.tool.Name != entry.name {
			t.Fatalf("descriptor %q advertises tool %q", entry.name, entry.tool.Name)
		}
		if entry.invoke == nil {
			t.Fatalf("%s has no invoke builder", entry.name)
		}
		advertised = append(advertised, entry.name)
		grouped[entry.requiredGroup] = append(grouped[entry.requiredGroup], entry.name)
	}
	if len(advertised) != len(slices.Compact(append([]string(nil), advertised...))) {
		t.Fatal("the catalogue contains a duplicate tool name")
	}
	if got, want := sortedCopy(grouped[""]), defaultToolNames(); !equalStrings(got, want) {
		t.Fatalf("default catalogue = %v, want %v", got, want)
	}

	planned := writeToolNames()
	got := make([]string, 0, len(planned))
	for name, group := range planned {
		entry := descriptorNamed(t, name)
		if entry.requiredGroup != group {
			t.Fatalf("%s requires group %q, want %q", name, entry.requiredGroup, group)
		}
		if entry.effect != effectWrite && entry.effect != effectProbe {
			t.Fatalf("%s has side effect %d, want a write or probe effect", name, entry.effect)
		}
		got = append(got, name)
	}
	groupedNames := make([]string, 0, len(grouped)-1)
	for group, names := range grouped {
		if group == "" {
			continue
		}
		groupedNames = append(groupedNames, names...)
	}
	if !equalStrings(sortedCopy(got), sortedCopy(groupedNames)) {
		t.Fatalf("grouped catalogue = %v, want %v", sortedCopy(groupedNames), sortedCopy(got))
	}
	if total := len(advertised); total != 24 {
		t.Fatalf("catalogue holds %d tools, want 24", total)
	}
}

func TestAdvertisedSchemasAreStrictObjects(t *testing.T) {
	session := sessionWithoutChild(t)
	advertised := session.mcpServer.ListTools()
	if len(advertised) != len(defaultToolNames()) {
		t.Fatalf("registered %d tools, want %d", len(advertised), len(defaultToolNames()))
	}
	for name, entry := range advertised {
		schema := entry.Tool.InputSchema
		if schema.Type != "object" {
			t.Fatalf("%s input schema type = %q, want object", name, schema.Type)
		}
		if schema.AdditionalProperties != false {
			t.Fatalf("%s does not reject unknown properties: additionalProperties=%v", name, schema.AdditionalProperties)
		}
	}
}

func TestDefaultToolsRejectUnknownArguments(t *testing.T) {
	session := sessionWithoutChild(t)
	for _, name := range defaultToolNames() {
		result := dispatchTool(t, session, name, map[string]any{"unexpected": true})
		if result.Success {
			t.Fatalf("%s accepted an unknown argument", name)
		}
		if result.Error.Code != CodeInvalidArguments {
			t.Fatalf("%s returned %q, want %q (a rejected argument must not reach execution)",
				name, result.Error.Code, CodeInvalidArguments)
		}
	}
}

func TestFixedArgumentVectors(t *testing.T) {
	ic := testInvocationContext(t)
	cases := []struct {
		name string
		args map[string]any
		want []string
	}{
		{"bridgectl_context_list", nil, []string{"-o", "json", "context", "list"}},
		{"bridgectl_api_list", nil, []string{"-o", "json", "api", "list"}},
		{"bridgectl_token_list", nil, []string{"-o", "json", "token", "list"}},
		{"bridgectl_cache_stats", nil, []string{"-o", "json", "cache", "stats"}},
		{"bridgectl_log_stats", nil, []string{"-o", "json", "log", "stats"}},
		{"bridgectl_tool_get", nil, []string{"-o", "json", "tool", "get", "-o", "json"}},
		{"bridgectl_tool_get", map[string]any{"name": "list_items"},
			[]string{"-o", "json", "tool", "get", "-o", "json", "--", "list_items"}},
		{"bridgectl_tool_get", map[string]any{"name": "list_items", "version": "1.2.0"},
			[]string{"-o", "json", "tool", "get", "-o", "json", "--", "list_items@1.2.0"}},
		{"bridgectl_plugin_get", nil, []string{"-o", "json", "plugin", "get", "-o", "json"}},
		{"bridgectl_plugin_get", map[string]any{"name": "redact", "version": "2.0.0"},
			[]string{"-o", "json", "plugin", "get", "-o", "json", "--", "redact@2.0.0"}},
		{"bridgectl_binding_get", nil, []string{"-o", "json", "plugin", "binding", "get", "-o", "json"}},
		{"bridgectl_binding_get", map[string]any{"name": "orders"},
			[]string{"-o", "json", "plugin", "binding", "get", "-o", "json", "--", "orders"}},
		{"bridgectl_tool_generate", map[string]any{"api": "erp"},
			[]string{"-o", "json", "tool", "generate", "--api", "erp"}},
	}
	for _, testCase := range cases {
		descriptor := descriptorNamed(t, testCase.name)
		inv, failure := descriptor.invoke(ic, testCase.args)
		if failure != nil {
			t.Fatalf("%s%v rejected valid arguments: %s", testCase.name, testCase.args, failure.Error.Message)
		}
		if !equalStrings(inv.argv, testCase.want) {
			t.Fatalf("%s%v argv = %v, want %v", testCase.name, testCase.args, inv.argv, testCase.want)
		}
		if inv.project == nil {
			t.Fatalf("%s has no projection", testCase.name)
		}
	}
}

func TestVersionedIdentifierRequiresNameForVersion(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_tool_get")
	_, failure := descriptor.invoke(ic, map[string]any{"version": "1.0.0"})
	if failure == nil || failure.Error.Code != CodeInvalidArguments {
		t.Fatalf("version without name was accepted: %+v", failure)
	}
}

func TestPluginGetRequiresVersionWhenNamed(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_plugin_get")
	_, failure := descriptor.invoke(ic, map[string]any{"name": "redact"})
	if failure == nil || failure.Error.Code != CodeInvalidArguments {
		t.Fatalf("plugin_get accepted a name without a version: %+v", failure)
	}
}

func TestIdentifiersRejectOptionInjection(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_tool_get")
	for _, value := range []string{"--force", "-o", "name\nwith-newline", strings.Repeat("x", maxIdentifierBytes+1)} {
		_, failure := descriptor.invoke(ic, map[string]any{"name": value})
		if failure == nil || failure.Error.Code != CodeInvalidArguments {
			t.Fatalf("identifier %q was accepted: %+v", value, failure)
		}
	}
}

func validToolManifest() map[string]any {
	return map[string]any{
		"apiVersion": "erpbridge.io/v1",
		"kind":       kindTool,
		"metadata":   map[string]any{"name": "list_items", "version": "1.0.0", "module": "erp"},
		"spec": map[string]any{
			"description": map[string]any{"short": "List items"},
			"execution":   map[string]any{"type": "http", "method": "GET", "endpoint": "https://erp.internal/api/items"},
			"security":    map[string]any{"authType": "api-key", "credentialRef": "ERP_PRIMARY_KEY"}, // #nosec G101 -- logical reference
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					// A declaration is legal; a concrete default for a
					// credential-like property would not be.
					"password": map[string]any{"type": "string", "description": "ERP password"},
				},
			},
		},
	}
}

func TestManifestToolsRejectUnsafeInput(t *testing.T) {
	ic := testInvocationContext(t)
	cases := []struct {
		label    string
		tool     string
		manifest any
	}{
		{"not an object", "bridgectl_tool_validate", "list_items"},
		{"array manifest", "bridgectl_tool_validate", []any{validToolManifest()}},
		{"wrong kind", "bridgectl_tool_validate", map[string]any{"kind": kindPlugin, "metadata": map[string]any{"name": "x"}}},
		{"wrong kind for plugin tool", "bridgectl_plugin_validate", map[string]any{"kind": kindTool, "metadata": map[string]any{"name": "x"}}},
		{"missing name", "bridgectl_tool_validate", map[string]any{"kind": kindTool}},
		{"literal credential", "bridgectl_tool_validate", map[string]any{
			"kind":     kindTool,
			"metadata": map[string]any{"name": "list_items"},
			"spec":     map[string]any{"security": map[string]any{"authType": "api-key", "authKey": "canary-secret-value"}},
		}},
		{"literal credential in plugin auth", "bridgectl_plugin_validate", map[string]any{
			"kind":     kindPlugin,
			"metadata": map[string]any{"name": "redact", "version": "1.0.0"},
			"spec":     map[string]any{"auth": map[string]any{"token": "canary-secret-value"}},
		}},
		{"literal default for a credential property", "bridgectl_tool_validate", map[string]any{
			"kind":     kindTool,
			"metadata": map[string]any{"name": "list_items", "version": "1.0.0"},
			"spec": map[string]any{
				"execution": map[string]any{"method": "GET", "endpoint": "https://erp.internal/api/items"},
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"password": map[string]any{"type": "string", "default": "canary-secret-value"},
					},
				},
			},
		}},
		{"credential-bearing endpoint", "bridgectl_tool_validate", map[string]any{
			"kind":     kindTool,
			"metadata": map[string]any{"name": "list_items", "version": "1.0.0"},
			"spec": map[string]any{
				"execution": map[string]any{"method": "GET", "endpoint": "https://user:canary-secret-value@erp.internal/api"}, // #nosec G101 -- canary userinfo
			},
		}},
		{"query-bearing endpoint", "bridgectl_plugin_validate", map[string]any{
			"kind":     kindPlugin,
			"metadata": map[string]any{"name": "redact", "version": "2.0.0"},
			"spec":     map[string]any{"endpoint": "https://plugins.internal/process?token=canary-secret-value"},
		}},
		{"wrong kind without a kind field", "bridgectl_binding_validate", map[string]any{
			"metadata": map[string]any{"name": "orders-redaction"},
		}},
		{"oversized manifest", "bridgectl_tool_validate", map[string]any{
			"kind":        kindTool,
			"metadata":    map[string]any{"name": "list_items"},
			"description": strings.Repeat("x", MaxManifestBytes),
		}},
	}
	for _, testCase := range cases {
		descriptor := descriptorNamed(t, testCase.tool)
		_, failure := descriptor.invoke(ic, map[string]any{"manifest": testCase.manifest})
		if failure == nil {
			t.Fatalf("%s: %s was accepted", testCase.tool, testCase.label)
		}
		if failure.Error.Code != CodeInvalidArguments {
			t.Fatalf("%s: %s returned %q, want %q", testCase.tool, testCase.label, failure.Error.Code, CodeInvalidArguments)
		}
		if strings.Contains(failure.Error.Message, "canary-secret-value") {
			t.Fatalf("%s: the failure message echoed a credential value", testCase.tool)
		}
	}
}

func TestSchemaDeclarationsSurviveCredentialScan(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_tool_validate")
	inv, failure := descriptor.invoke(ic, map[string]any{"manifest": validToolManifest()})
	if failure != nil {
		t.Fatalf("a declared password property was rejected: %s", failure.Error.Message)
	}
	argv := inv.argv
	if len(argv) < 2 || argv[len(argv)-2] != "-f" {
		t.Fatalf("argv = %v, want a -f staging path", argv)
	}
	path := argv[len(argv)-1]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("staged manifest is missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("staged manifest mode = %o, want 600", perm)
	}

	data, err := inv.project(nil)
	if err != nil {
		t.Fatalf("validation projection: %v", err)
	}
	summary, ok := data.(ValidationData)
	if !ok {
		t.Fatalf("validation projection is %T", data)
	}
	if summary.Kind != kindTool || summary.Name != "list_items" || summary.Version != "1.0.0" || !summary.Valid {
		t.Fatalf("validation summary = %+v", summary)
	}
}

func TestProjectionsDropSensitiveFields(t *testing.T) {
	cases := []struct {
		name     string
		project  func([]byte) (any, error)
		stdout   string
		canaries []string
		assert   func(*testing.T, any)
	}{
		{
			name:     "context list",
			project:  projectContextList,
			stdout:   `{"items":[{"name":"test","server":"http://operator:secret@ctx.internal:9443","current":true}]}`, // #nosec G101 -- canary userinfo
			canaries: []string{"operator", "secret", "ctx.internal"},
			assert: func(t *testing.T, data any) {
				list := data.(ContextListData)
				if len(list.Items) != 1 || list.Items[0].Name != "test" || !list.Items[0].SelectedForSession {
					t.Fatalf("context projection = %+v", list)
				}
			},
		},
		{
			name:    "api list",
			project: projectAPIList,
			stdout:  `{"items":[{"id":"a1","name":"erp","url":"https://admin:hunter2@erp.internal:8443/api/v1?token=canary","method":"GET","module":"erp","description":"Registered","status":"active","authType":"api-key","authHeader":"X-API-Key","credentialRef":"ERP_PRIMARY_KEY","credentialSource":"env","authUsername":"admin"}]}`, // #nosec G101 -- canary userinfo
			canaries: []string{
				"hunter2", "canary", "authUsername", "admin:", "/api/v1",
			},
			assert: func(t *testing.T, data any) {
				list := data.(APIListData)
				if len(list.Items) != 1 {
					t.Fatalf("api projection = %+v", list)
				}
				item := list.Items[0]
				if item.EndpointLabel != "erp.internal:8443" {
					t.Fatalf("endpointLabel = %q, want the neutral host label", item.EndpointLabel)
				}
				if item.CredentialRef != "ERP_PRIMARY_KEY" || item.AuthType != "api-key" {
					t.Fatalf("logical credential metadata was dropped: %+v", item)
				}
			},
		},
		{
			name:    "tool list",
			project: projectToolList,
			stdout: encodeFor(t, map[string]any{"tools": []any{map[string]any{
				"apiVersion": "erpbridge.io/v1",
				"kind":       kindTool,
				"metadata":   map[string]any{"name": "list_items", "version": "1.0.0", "module": "erp", "isActive": true},
				"spec": map[string]any{
					"description": map[string]any{"short": "List items"},
					"execution": map[string]any{
						"method": "GET", "endpoint": "https://erp.internal/api/items?apikey=canary", // #nosec G101 -- canary query value
					},
					"security": map[string]any{ // #nosec G101 -- logical references, not values
						"authType": "api-key", "authHeader": "X-API-Key", "credentialRef": "ERP_PRIMARY_KEY",
						"credentialSource": "env",
					},
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []any{"password"},
						"properties": map[string]any{
							"password": map[string]any{
								"type": "string", "description": "ERP password", "default": "canary-default",
								"enum": []any{"one", "two"},
							},
						},
					},
				},
			}}}),
			canaries: []string{"canary", "/api/items"},
			assert: func(t *testing.T, data any) {
				list := data.(ResourceData)
				if !list.Redacted || len(list.Items) != 1 {
					t.Fatalf("tool projection = %+v", list)
				}
				item := list.Items[0]
				if item.EndpointLabel != "erp.internal" {
					t.Fatalf("endpointLabel = %q", item.EndpointLabel)
				}
				property, present := item.InputProperties["password"]
				if !present {
					t.Fatal("a declared password property was dropped from the schema")
				}
				if !property.Required || property.Type != "string" || len(property.Enum) != 2 {
					t.Fatalf("schema declaration was not preserved: %+v", property)
				}
			},
		},
		{
			name:     "token list",
			project:  projectTokenList,
			stdout:   `{"items":[{"id":"t1","name":"agent","scopes":["mcp"],"roles":["viewer"],"createdAt":"2026-01-02T03:04:05Z","token":"canary-token","secretHash":"canary-hash"}]}`,
			canaries: []string{"canary-token", "canary-hash", "secretHash", "token\":"},
			assert: func(t *testing.T, data any) {
				list := data.(TokenListData)
				if len(list.Items) != 1 || list.Items[0].ID != "t1" || list.Items[0].Name != "agent" {
					t.Fatalf("token projection = %+v", list)
				}
			},
		},
		{
			name:    "cache stats",
			project: projectCacheStats,
			stdout:  `{"stats":{"exactKeys":42,"redisMemory":"1.5MB"}}`,
			assert: func(t *testing.T, data any) {
				stats := data.(CacheStatsData)
				if stats.ExactKeys != 42 || stats.RedisMemory != "1.5MB" {
					t.Fatalf("cache projection = %+v", stats)
				}
			},
		},
		{
			name:    "log stats",
			project: projectLogStats,
			stdout:  `{"levelCounts":{"info":3,"weird":1},"toolCounts":{"list_items":2,"bad\u0001label":1}}`,
			assert: func(t *testing.T, data any) {
				stats := data.(LogStatsData)
				if stats.LevelCounts["info"] != 3 || stats.LevelCounts[unrecognizedLabel] != 1 {
					t.Fatalf("level counts = %v", stats.LevelCounts)
				}
				if stats.ToolCounts["list_items"] != 2 || stats.ToolCounts[unrecognizedLabel] != 1 {
					t.Fatalf("tool counts = %v", stats.ToolCounts)
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := testCase.project([]byte(testCase.stdout))
			if err != nil {
				t.Fatalf("projection failed: %v", err)
			}
			assertNoSecret(t, data, testCase.canaries...)
			if testCase.assert != nil {
				testCase.assert(t, data)
			}
		})
	}
}

func TestProjectionsRejectMalformedChildOutput(t *testing.T) {
	cases := []struct {
		name    string
		project func([]byte) (any, error)
		stdout  string
	}{
		{"empty", projectContextList, ""},
		{"not json", projectContextList, "not-json"},
		{"two documents", projectContextList, `{"items":[]}{"items":[]}`},
		{"wrong field type", projectContextList, `{"items":{}}`},
		{"missing name", projectAPIList, `{"items":[{"id":"a1"}]}`},
		{"missing token id", projectTokenList, `{"items":[{"name":"agent"}]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := testCase.project([]byte(testCase.stdout)); err == nil {
				t.Fatal("malformed child output was accepted")
			}
		})
	}
}

func TestGeneratedManifestSafety(t *testing.T) {
	safe := map[string]any{
		"apiVersion": "erpbridge.io/v1",
		"kind":       kindTool,
		"metadata":   map[string]any{"name": "list_items", "version": "1.0.0"},
		"spec": map[string]any{
			"execution": map[string]any{"method": "GET", "endpoint": "https://erp.internal/api/items"},
			"security":  map[string]any{"authType": "api-key", "credentialRef": "ERP_PRIMARY_KEY"}, // #nosec G101 -- logical reference
		},
	}
	data, err := projectGeneratedTool([]byte(encodeFor(t, safe)))
	if err != nil {
		t.Fatalf("a safe generated manifest was rejected: %v", err)
	}
	generated := data.(GenerateData)
	if len(generated.Manifests) != 1 {
		t.Fatalf("generated manifests = %d, want 1", len(generated.Manifests))
	}

	unsafe := []struct {
		label  string
		mutate func(map[string]any)
	}{
		{"credential value", func(document map[string]any) {
			document["spec"].(map[string]any)["security"].(map[string]any)["authKey"] = "canary-secret"
		}},
		{"userinfo endpoint", func(document map[string]any) {
			document["spec"].(map[string]any)["execution"].(map[string]any)["endpoint"] = "https://user:pass@erp.internal/api"
		}},
		{"query endpoint", func(document map[string]any) {
			document["spec"].(map[string]any)["execution"].(map[string]any)["endpoint"] = "https://erp.internal/api?key=canary"
		}},
		{"non-http endpoint", func(document map[string]any) {
			document["spec"].(map[string]any)["execution"].(map[string]any)["endpoint"] = "file:///etc/passwd"
		}},
	}
	for _, testCase := range unsafe {
		t.Run(testCase.label, func(t *testing.T) {
			document := map[string]any{
				"apiVersion": safe["apiVersion"],
				"kind":       safe["kind"],
				"metadata":   map[string]any{"name": "list_items", "version": "1.0.0"},
				"spec": map[string]any{
					"execution": map[string]any{"method": "GET", "endpoint": "https://erp.internal/api/items"},
					"security":  map[string]any{"authType": "api-key", "credentialRef": "ERP_PRIMARY_KEY"}, // #nosec G101 -- logical reference // #nosec G101 -- logical reference
				},
			}
			testCase.mutate(document)
			if _, err := projectGeneratedTool([]byte(encodeFor(t, document))); err == nil {
				t.Fatal("an unsafe generated manifest was returned")
			}
		})
	}

	if _, err := projectGeneratedTool([]byte("[]")); err == nil {
		t.Fatal("a non-object generated manifest was accepted")
	}
}

func TestExitFailureClassification(t *testing.T) {
	session := sessionWithoutChild(t)
	descriptor := descriptorNamed(t, "bridgectl_api_list")

	cases := []struct {
		label    string
		outcome  Outcome
		wantCode string
	}{
		{"recognized envelope on stdout", Outcome{ExitCode: 3, Stdout: []byte(`{"error":"CONTEXT_NOT_FOUND","message":"leaky text"}`)}, CodeNotFound},
		{"recognized envelope on stderr", Outcome{ExitCode: 4, Stderr: []byte(`{"error":"AUTHORIZATION_DENIED"}`)}, CodeAuthorizationDenied},
		{"exit code fallback", Outcome{ExitCode: 5}, CodeConflict},
		{"unknown error text", Outcome{ExitCode: 1, Stderr: []byte("some internal failure detail")}, CodeInternal},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			result := session.classifyExitFailure(descriptor, testCase.outcome)
			if result.Error.Code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", result.Error.Code, testCase.wantCode)
			}
			if strings.Contains(result.Error.Message, "leaky") || strings.Contains(result.Error.Message, "internal failure detail") {
				t.Fatalf("child text leaked into the message: %q", result.Error.Message)
			}
		})
	}
}

func TestLocalValidationFailureIsAnInputError(t *testing.T) {
	session := sessionWithoutChild(t)
	descriptor := descriptorNamed(t, "bridgectl_tool_validate")
	result := session.classifyExitFailure(descriptor, Outcome{ExitCode: 1, Stderr: []byte("validation failed: metadata.version is missing")})
	if result.Error.Code != CodeInvalidArguments {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeInvalidArguments)
	}
	if strings.Contains(result.Error.Message, "metadata.version") {
		t.Fatalf("validation detail leaked into the message: %q", result.Error.Message)
	}
}

func TestReadFailuresStaySpecific(t *testing.T) {
	session := sessionWithoutChild(t)
	descriptor := descriptorNamed(t, "bridgectl_api_list")
	cases := []struct {
		err      error
		wantCode string
	}{
		{ErrBusy, CodeBusy},
		{ErrOutputLimit, CodeOutputLimit},
		{ErrTimeout, CodeTimeout},
		{ErrCancelled, CodeCancelled},
		{os.ErrNotExist, CodeInternal},
	}
	for _, testCase := range cases {
		result := session.classifyRunFailure(descriptor, Outcome{}, testCase.err)
		if result.Error.Code != testCase.wantCode {
			t.Fatalf("%v mapped to %q, want %q", testCase.err, result.Error.Code, testCase.wantCode)
		}
	}
}

func TestProjectionFailureIsNotReportedAsSuccess(t *testing.T) {
	session := sessionWithoutChild(t)
	descriptor := descriptorNamed(t, "bridgectl_api_list")
	inv, failure := descriptor.invoke(testInvocationContext(t), nil)
	if failure != nil {
		t.Fatalf("invoke: %s", failure.Error.Message)
	}
	result := session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: []byte("not json")}, nil)
	if result.Success {
		t.Fatal("unparseable child output was reported as success")
	}
	if result.Error.Code != CodeInvalidChildOutput {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeInvalidChildOutput)
	}
}

func TestDisabledGroupIsRejectedAtDispatch(t *testing.T) {
	session := sessionWithoutChild(t)
	descriptor := descriptorNamed(t, "bridgectl_api_list")
	descriptor.requiredGroup = GroupRegistry
	result := decodeEnvelope(t, mustDispatch(t, session, descriptor, nil))
	if result.Success || result.Error.Code != CodePermissionDenied {
		t.Fatalf("ungrouped dispatch = %+v", result)
	}
}

func mustDispatch(t *testing.T, session *Server, descriptor descriptor, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	call, err := session.dispatch(context.Background(), descriptor, args)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	return call
}
