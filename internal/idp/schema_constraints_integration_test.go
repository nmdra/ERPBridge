package idp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestSchemaConstraintGeneratedStoredDiscoveryAndInvocation(t *testing.T) {
	t.Setenv("API_AUTH_TOKEN", "schema-test-admin")
	var calls atomic.Int64
	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer erp.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.yaml")
	spec := `openapi: 3.0.3
info: {title: Constraints, version: 1.0.0}
servers: [{url: 'ERP_URL'}]
paths:
  /draft/{id}:
    post:
      operationId: constrained_draft
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
        - {name: limit, in: query, schema: {type: integer}}
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: false
              required: [docstatus, synchronous, company, detail, rows]
              properties:
                docstatus: {type: integer, enum: [0]}
                synchronous: {type: boolean, enum: [true]}
                company: {type: string, enum: [Approved Company]}
                detail:
                  type: object
                  additionalProperties: false
                  properties:
                    state: {type: string, enum: [draft]}
                rows:
                  type: array
                  items:
                    type: object
                    additionalProperties: false
                    properties:
                      amount: {type: number}
      responses:
        '200': {description: OK}
`
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(spec, "ERP_URL", erp.URL)), 0600))
	tools, err := NewGenerator(dir, logger.Init()).GenerateFromOpenAPI(context.Background(), API{Name: "fixture"}, path)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	encoded, err := json.Marshal(tools[0])
	require.NoError(t, err)
	var manifest mcp.Tool
	require.NoError(t, json.Unmarshal(encoded, &manifest))
	store, err := mcp.NewStore(filepath.Join(dir, "tools.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	require.NoError(t, store.Save(&manifest))
	stored, err := store.Get(manifest.Metadata.Name, manifest.Metadata.Version)
	require.NoError(t, err)
	// Persistence is verified through tools.db above. The invocation server has
	// no public store-close API, so avoid a live file handle during TempDir cleanup.
	s := mcp.NewServer(connector.NewClient(logger.Init()), nil, logger.Init(), mcp.RateLimitConfig{RequestsPerSecond: 1000, Burst: 1000}, ":memory:")
	s.RegisterTool(stored)
	listed := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)).(protocol.JSONRPCResponse)
	wire, err := json.Marshal(listed.Result)
	require.NoError(t, err)
	var discovery struct {
		Tools []struct {
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(wire, &discovery))
	require.NotEmpty(t, discovery.Tools)
	schema := discovery.Tools[0].InputSchema
	require.Equal(t, false, schema["additionalProperties"])
	props := schema["properties"].(map[string]any)
	require.Equal(t, []any{float64(0)}, props["docstatus"].(map[string]any)["enum"])
	require.Equal(t, []any{true}, props["synchronous"].(map[string]any)["enum"])
	require.Equal(t, false, props["detail"].(map[string]any)["additionalProperties"])
	require.Equal(t, false, props["rows"].(map[string]any)["items"].(map[string]any)["additionalProperties"])
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")
	valid := `{"id":"draft-1","limit":1,"docstatus":0,"synchronous":true,"company":"Approved Company","detail":{"state":"draft"},"rows":[{"amount":1.5}]}`
	cases := []struct {
		name, args string
		valid      bool
	}{
		{"valid", valid, true},
		{"integer enum", strings.Replace(valid, `"docstatus":0`, `"docstatus":1`, 1), false},
		{"string integer", strings.Replace(valid, `"docstatus":0`, `"docstatus":"0"`, 1), false},
		{"boolean enum", strings.Replace(valid, `"synchronous":true`, `"synchronous":false`, 1), false},
		{"string boolean", strings.Replace(valid, `"synchronous":true`, `"synchronous":"true"`, 1), false},
		{"required", strings.Replace(valid, `"docstatus":0,`, ``, 1), false},
		{"company", strings.Replace(valid, `Approved Company`, `private-input-marker`, 1), false},
		{"string enum", strings.Replace(valid, `"state":"draft"`, `"state":"private-input-marker"`, 1), false},
		{"cmd override", strings.Replace(valid, `"limit":1`, `"limit":1,"cmd":"harmless.private-input-marker"`, 1), false},
		{"nested unknown", strings.Replace(valid, `"state":"draft"`, `"state":"draft","extra":"private-input-marker"`, 1), false},
		{"array unknown", strings.Replace(valid, `"amount":1.5`, `"amount":1.5,"extra":"private-input-marker"`, 1), false},
	}
	for _, transport := range []string{"MCP", "direct"} {
		for _, tc := range cases {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				before := calls.Load()
				var response []byte
				if transport == "MCP" {
					request := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + stored.Metadata.Name + `","arguments":` + tc.args + `}}`)
					rpc, ok := s.MCPServer().HandleMessage(context.Background(), request).(protocol.JSONRPCResponse)
					require.True(t, ok)
					result := rpc.Result.(*protocol.CallToolResult)
					require.Equal(t, !tc.valid, result.IsError)
					response, err = json.Marshal(result)
					require.NoError(t, err)
				} else {
					request := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(`{"name":"`+stored.Metadata.Name+`@`+stored.Metadata.Version+`","arguments":`+tc.args+`}`))
					request.Header.Set("Authorization", "Bearer schema-test-admin")
					recorder := httptest.NewRecorder()
					mux.ServeHTTP(recorder, request)
					require.Equal(t, tc.valid, recorder.Code == http.StatusOK, recorder.Body.String())
					response = recorder.Body.Bytes()
				}
				if tc.valid {
					require.Equal(t, before+1, calls.Load())
				} else {
					require.Equal(t, before, calls.Load())
					require.NotContains(t, string(response), "private-input-marker")
					require.NotContains(t, string(response), "schema-test-admin")
				}
			})
		}
	}
}
