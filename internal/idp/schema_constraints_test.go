package idp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestGeneratorTypedEnums(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typed-enums.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`openapi: 3.0.3
info: {title: Typed enum regression, version: 1.0.0}
servers: [{url: 'https://erp.example.test/api'}]
paths:
  /draft:
    post:
      operationId: create_draft
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [docstatus, synchronous]
              properties:
                docstatus: {type: integer, enum: [0]}
                synchronous: {type: boolean, enum: [true]}
                rate: {type: number, enum: [1.5]}
                state: {type: string, enum: [draft]}
                payload: {type: object, enum: [{state: draft, flags: [true, 0]}]}
                choices: {type: array, items: {type: integer}, enum: [[0, 1]]}
      responses:
        '200': {description: OK}
`), 0600))
	tools, err := NewGenerator(dir, logger.Init()).GenerateFromOpenAPI(context.Background(), API{Name: "fixture"}, path)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	// Assert the manifest's public wire values, not the Go representation.
	encoded, err := json.Marshal(tools[0])
	require.NoError(t, err)
	var tool map[string]any
	require.NoError(t, json.Unmarshal(encoded, &tool))
	properties := tool["spec"].(map[string]any)["inputSchema"].(map[string]any)["properties"].(map[string]any)
	for name, expected := range map[string]any{"docstatus": float64(0), "synchronous": true, "rate": 1.5, "state": "draft", "payload": map[string]any{"state": "draft", "flags": []any{true, float64(0)}}, "choices": []any{float64(0), float64(1)}} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, []any{expected}, properties[name].(map[string]any)["enum"])
		})
	}
}

func TestGeneratorAdditionalProperties(t *testing.T) {
	for _, policy := range []string{"false", "true", "{type: integer, enum: [0]}", "omitted"} {
		t.Run(policy, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "closure.yaml")
			spec := `openapi: 3.0.3
info: {title: Closure, version: 1.0.0}
paths:
  /draft/{id}:
    post:
      operationId: closed_input
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
        - {name: limit, in: query, schema: {type: integer}}
      requestBody:
        content:
          application/json:
            schema:
              type: object
              additionalProperties: POLICY
              properties:
                detail:
                  type: object
                  additionalProperties: POLICY
                rows:
                  type: array
                  items: {type: object, additionalProperties: POLICY}
      responses:
        '200': {description: OK}
`
			spec = strings.ReplaceAll(spec, "POLICY", policy)
			if policy == "omitted" {
				spec = strings.ReplaceAll(spec, "additionalProperties: omitted", "")
				spec = strings.ReplaceAll(spec, "type: object, }", "type: object}")
			}
			require.NoError(t, os.WriteFile(path, []byte(spec), 0600))
			tools, err := NewGenerator(dir, logger.Init()).GenerateFromOpenAPI(context.Background(), API{Name: "fixture"}, path)
			require.NoError(t, err)
			encoded, err := json.Marshal(tools[0])
			require.NoError(t, err)
			var restored mcp.Tool
			require.NoError(t, json.Unmarshal(encoded, &restored))
			encoded, err = json.Marshal(restored)
			require.NoError(t, err)
			var tool map[string]any
			require.NoError(t, json.Unmarshal(encoded, &tool))
			schema := tool["spec"].(map[string]any)["inputSchema"].(map[string]any)
			expected := map[string]any{"false": false, "true": true, "{type: integer, enum: [0]}": map[string]any{"type": "integer", "enum": []any{float64(0)}}}[policy]
			require.Equal(t, expected, schema["additionalProperties"])
			props := schema["properties"].(map[string]any)
			require.Contains(t, props, "id")
			require.Contains(t, props, "limit")
			require.Equal(t, expected, props["detail"].(map[string]any)["additionalProperties"])
			require.Equal(t, expected, props["rows"].(map[string]any)["items"].(map[string]any)["additionalProperties"])
		})
	}
}
