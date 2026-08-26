package idp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_GenerateFromOpenAPI(t *testing.T) {
	log := logger.Init()
	tempDir, err := os.MkdirTemp("", "schemas")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	gen := NewGenerator(tempDir, log)

	// Create a dummy OpenAPI file
	spec := `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
servers:
  - url: http://localhost:8081
paths:
  /test:
    get:
      operationId: getTest
      summary: Get test data
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  foo: {type: string}
`
	specPath := filepath.Join(tempDir, "spec.yaml")
	err = os.WriteFile(specPath, []byte(spec), 0600)
	assert.NoError(t, err)

	api := API{
		Name:          "test",
		Module:        "finance",
		AuthType:      "bearer",
		CredentialRef: "ERP_OPENAPI_KEY", // #nosec G101 -- environment-variable reference, not a secret.
	}

	tools, err := gen.GenerateFromOpenAPI(context.Background(), api, specPath)
	assert.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, "gettest", tools[0].Metadata.Name)
	assert.Equal(t, "Get test data", tools[0].Spec.Description.Short)
	assert.Equal(t, []string{"Use when the user asks for: Get test data"}, tools[0].Spec.Description.WhenToUse)
	assert.Equal(t, []string{"I need help with: Get test data"}, tools[0].Spec.Description.Examples)
	assert.Equal(t, "ERP_OPENAPI_KEY", tools[0].Spec.Security.CredentialRef)
	assert.Equal(t, "GET", tools[0].Spec.Execution.Method)
	assert.Equal(t, &cache.Config{Enabled: true, TTLSeconds: 300, IsReadOnly: true, FlushOn: []string{}}, tools[0].Spec.Cache)

	// Generation is pure. It must not create implicit sibling artifacts.
	_, err = os.Stat(filepath.Join(tempDir, "finance"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGenerator_RequiresAndUsesCredentialReference(t *testing.T) {
	gen := NewGenerator(t.TempDir(), logger.Init())
	_, err := gen.Generate(API{Name: "secured", AuthType: "api-key"})
	assert.Error(t, err)
	// #nosec G101 -- this is an environment-variable reference used by the test.
	api := API{Name: "secured", AuthType: "api-key", CredentialRef: "ERP_CUSTOM_KEY"}
	tool, err := gen.Generate(api)
	assert.NoError(t, err)
	assert.Equal(t, "ERP_CUSTOM_KEY", tool.Spec.Security.CredentialRef)
}

func TestGenerator_GenerateIsPureAndSaveIsExplicit(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir, logger.Init())
	tool, err := gen.Generate(API{Name: "simple-test", Module: "hr", Method: "GET", URL: "/hr/test"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "hr", "simple-test.json"))
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, gen.Save(tool))
	_, err = os.Stat(filepath.Join(dir, "hr", "simple-test.json"))
	require.NoError(t, err)
}

func TestGenerator_Generate(t *testing.T) {
	log := logger.Init()
	tempDir, err := os.MkdirTemp("", "schemas")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	gen := NewGenerator(tempDir, log)

	api := API{
		Name:        "simple-test",
		Description: "A simple test",
		Module:      "hr",
		Method:      "GET",
		URL:         "/hr/test",
	}

	tool, err := gen.Generate(api)
	assert.NoError(t, err)
	assert.Equal(t, "simple-test", tool.Metadata.Name)
	assert.Equal(t, "A simple test", tool.Spec.Description.Short)
	assert.Equal(t, []string{"Use when the user asks for: A simple test"}, tool.Spec.Description.WhenToUse)
	assert.Equal(t, []string{"I need help with: A simple test"}, tool.Spec.Description.Examples)
	assert.Equal(t, &cache.Config{Enabled: true, TTLSeconds: 300, IsReadOnly: true, FlushOn: []string{}}, tool.Spec.Cache)
}

func TestGenerator_MethodAwareCacheDefaults(t *testing.T) {
	gen := NewGenerator(t.TempDir(), logger.Init())
	for _, method := range []string{"HEAD", "head"} {
		tool, err := gen.Generate(API{Name: "head-check", Method: method})
		require.NoError(t, err)
		require.Equal(t, "HEAD", tool.Spec.Execution.Method)
		require.Equal(t, &cache.Config{Enabled: true, TTLSeconds: 300, IsReadOnly: true, FlushOn: []string{}}, tool.Spec.Cache)
	}

	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		tool, err := gen.Generate(API{Name: "write-check-" + method, Method: method})
		require.NoError(t, err)
		require.Equal(t, &cache.Config{Enabled: false, TTLSeconds: 0, IsReadOnly: false, FlushOn: []string{}}, tool.Spec.Cache)
	}
}

func TestGenerator_GenerateFromOpenAPICollisionsFailBeforeReturningTools(t *testing.T) {
	spec := `
openapi: 3.0.0
info:
  title: Collision API
  version: 1.0.0
paths:
  /orders:
    get:
      operationId: list-orders
      summary: List orders
      responses:
        '200': {description: OK}
  /orders/{id}:
    get:
      operationId: list_orders
      summary: Get one order
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        '200': {description: OK}
`
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(spec), 0600))

	gen := NewGenerator(t.TempDir(), logger.Init())
	tools, err := gen.GenerateFromOpenAPI(context.Background(), API{Module: "sales"}, path)
	require.Error(t, err)
	require.Nil(t, tools)
	require.Contains(t, err.Error(), "generated tool name collision")
	require.Contains(t, err.Error(), "list_orders")
}

func TestGenerator_OpenAPIStructureAndDeterminism(t *testing.T) {
	spec := `
openapi: 3.0.0
info:
  title: Structure API
  version: 1.0.0
servers:
  - url: http://localhost:8081
paths:
  /orders/{id}:
    get:
      operationId: getOrder
      summary: Get an order
      description: Read one order by its identifier.
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: {type: object, properties: {id: {type: string}}}
  /orders:
    post:
      operationId: createOrder
      summary: Create an order
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [customer]
              properties:
                customer: {type: string}
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema: {type: object, properties: {id: {type: string}}}
`
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(spec), 0600))
	api := API{Module: "sales", AuthType: "bearer", CredentialRef: "ERP_API_KEY"} // #nosec G101 -- environment-variable reference.

	first, err := NewGenerator(t.TempDir(), logger.Init()).GenerateFromOpenAPI(context.Background(), api, path)
	require.NoError(t, err)
	second, err := NewGenerator(t.TempDir(), logger.Init()).GenerateFromOpenAPI(context.Background(), api, path)
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, "createorder", first[0].Metadata.Name)
	require.Equal(t, "getorder", first[1].Metadata.Name)
	require.Equal(t, first[0], second[0])
	require.Equal(t, first[1], second[1])

	get := first[1]
	require.Equal(t, "erpbridge.io/v1", get.APIVersion)
	require.Equal(t, "MCPTool", get.Kind)
	require.Equal(t, "sales", get.Metadata.Module)
	require.Equal(t, "object", get.Spec.InputSchema.Type)
	require.Equal(t, "http", get.Spec.Execution.Type)
	require.Equal(t, "GET", get.Spec.Execution.Method)
	require.Equal(t, "http://localhost:8081/orders/{id}", get.Spec.Execution.Endpoint)
	require.Equal(t, "data", get.Spec.Execution.ResponsePath)
	require.Equal(t, "string", get.Spec.InputSchema.Properties["id"].Type)
	require.Equal(t, []string{"id"}, get.Spec.InputSchema.Required)
	require.NotNil(t, get.Spec.OutputSchema)
	require.Equal(t, "ERP_API_KEY", get.Spec.Security.CredentialRef)

	create := first[0]
	require.Equal(t, "POST", create.Spec.Execution.Method)
	require.Equal(t, []string{"customer"}, create.Spec.InputSchema.Required)
	require.False(t, create.Spec.Cache.Enabled)
	require.False(t, create.Spec.Cache.IsReadOnly)
	require.NotEmpty(t, create.Spec.Description.WhenToUse)
	require.NotEmpty(t, create.Spec.Description.Examples)
}

func TestDereferenceSchema_RemovesRefsAndRejectsUnknownRefs(t *testing.T) {
	component := openapi3.NewStringSchema()
	doc := &openapi3.T{Components: &openapi3.Components{
		Schemas: openapi3.Schemas{
			"Thing": &openapi3.SchemaRef{Ref: "#/components/schemas/Thing", Value: component},
		},
	}}
	root := openapi3.NewObjectSchema()
	root.Properties["thing"] = &openapi3.SchemaRef{Ref: "#/components/schemas/Thing", Value: component}

	resolved, err := dereferenceSchema(doc, root)
	assert.NoError(t, err)
	data, err := json.Marshal(resolved)
	assert.NoError(t, err)
	assert.NotContains(t, string(data), `"$ref"`)

	root.Properties["missing"] = &openapi3.SchemaRef{Ref: "#/components/schemas/Missing"}
	_, err = dereferenceSchema(doc, root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved reference")
}
