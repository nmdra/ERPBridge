package mcp

import (
	"encoding/json"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAmountField  = "amount"
	testRoleAlpha    = "alpha"
	testRoleAdmin    = "admin"
	testRoleOperator = "operator"
	testRoleZeta     = "zeta"
	testClientOne    = "client-1"
)

func TestValidateToolDataClassRequiresRoles(t *testing.T) {
	s := &Server{}
	base := func(dataClass string, roles ...string) *Tool {
		return &Tool{
			Metadata: Metadata{Name: "employee-tool", Version: testVersion100},
			Spec: ToolSpec{
				InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{}},
				Security:    Security{DataClass: dataClass, AllowedRoles: roles},
			},
		}
	}

	for _, dataClass := range []string{"", "public", "internal", "pii", "restricted"} {
		t.Run("accepts_"+dataClass, func(t *testing.T) {
			roles := []string(nil)
			if dataClass == "pii" || dataClass == "restricted" {
				roles = []string{testRoleAlpha}
			}
			require.NoError(t, s.validateTool(base(dataClass, roles...)))
		})
	}

	for _, dataClass := range []string{"pii", "restricted"} {
		t.Run("rejects_"+dataClass+"_without_roles", func(t *testing.T) {
			err := s.validateTool(base(dataClass))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "allowedRoles")
		})
	}

	err := s.validateTool(base("secret"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataClass")
}

func TestValidateToolAllowedRoles(t *testing.T) {
	s := &Server{}
	tool := &Tool{
		Metadata: Metadata{Name: "role-tool", Version: testVersion100},
		Spec: ToolSpec{
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{testAmountField: {Type: schemaTypeNumber}}},
			Security:    Security{AllowedRoles: []string{testRoleZeta, testRoleAlpha}},
		},
	}
	require.NoError(t, s.validateTool(tool))
	assert.Equal(t, []string{testRoleAlpha, testRoleZeta}, tool.Spec.Security.AllowedRoles)

	tool.Spec.Security.AllowedRoles = []string{testRoleOperator}
	tool.Spec.InputSchema.Properties[roleSelectorField] = Property{Type: schemaTypeString}
	assert.Error(t, s.validateTool(tool))

	tool.Spec.Security.AllowedRoles = []string{"Admin"}
	delete(tool.Spec.InputSchema.Properties, roleSelectorField)
	assert.Error(t, s.validateTool(tool))
}

func TestSchemaForMCP_InjectsGuardedRoleSelectorWithoutMutatingTool(t *testing.T) {
	tool := &Tool{
		Metadata: Metadata{Name: "role-tool", Version: testVersion100},
		Spec: ToolSpec{
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{testAmountField: {Type: schemaTypeNumber}}},
			Security:    Security{AllowedRoles: []string{testRoleZeta, testRoleAlpha}},
		},
	}

	schema, err := schemaForMCP(tool)
	require.NoError(t, err)
	assert.Equal(t, []string{testRoleAlpha, testRoleZeta}, schema.Properties[roleSelectorField].Enum)
	assert.NotContains(t, tool.Spec.InputSchema.Properties, roleSelectorField)

	schema.Properties[roleSelectorField] = Property{Type: "integer"}
	assert.Equal(t, []string{"number"}, []string{tool.Spec.InputSchema.Properties[testAmountField].Type})
}

func TestSchemaForMCP_PreservesOpenBusinessRole(t *testing.T) {
	tool := &Tool{
		Metadata: Metadata{Name: "open-tool", Version: testVersion100},
		Spec: ToolSpec{
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{
				roleSelectorField: {Type: schemaTypeString, Description: "ERP business role"},
			}},
		},
	}

	schema, err := schemaForMCP(tool)
	require.NoError(t, err)
	assert.Equal(t, "ERP business role", schema.Properties[roleSelectorField].Description)
	assert.Empty(t, schema.Properties[roleSelectorField].Enum)
}

func TestRegisterTool_ExposesGuardedRoleSelectorToMCP(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	tool := &Tool{
		Metadata: Metadata{Name: "listed-role-tool", Version: testVersion100},
		Spec: ToolSpec{
			InputSchema: InputSchema{Type: schemaTypeObject, Properties: map[string]Property{testAmountField: {Type: schemaTypeNumber}}},
			Security:    Security{AllowedRoles: []string{testRoleOperator, testRoleAdmin}},
		},
	}

	s.RegisterTool(tool)
	registered := s.MCPServer().GetTool("listed-role-tool")
	require.NotNil(t, registered)
	encoded, err := json.Marshal(registered.Tool)
	require.NoError(t, err)
	var listed map[string]any
	require.NoError(t, json.Unmarshal(encoded, &listed))
	inputSchema, ok := listed["inputSchema"].(map[string]any)
	require.True(t, ok)
	properties, ok := inputSchema["properties"].(map[string]any)
	require.True(t, ok)
	role, ok := properties[roleSelectorField].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{testRoleAdmin, testRoleOperator}, role["enum"])
}
