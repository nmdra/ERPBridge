package web

import (
	"testing"

	"github.com/nmdra/ERPBridge/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestEnumProjectionKeepsDisplayLabels(t *testing.T) {
	tool := mcp.Tool{Spec: mcp.ToolSpec{InputSchema: mcp.InputSchema{Properties: map[string]mcp.Property{
		"value": {Enum: []any{"draft", 0, 1.5, true, map[string]any{"state": "draft"}, []any{0, false}}},
	}}}}
	projection := projectToolManifest(tool)
	require.Equal(t, []string{"draft", "0", "1.5", "true", `{"state":"draft"}`, `[0,false]`}, projection.InputFields[0].Enum)
	require.Equal(t, []any{"draft", 0, 1.5, true, map[string]any{"state": "draft"}, []any{0, false}}, tool.Spec.InputSchema.Properties["value"].Enum)
}
