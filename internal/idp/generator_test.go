package idp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, "ERP_OPENAPI_KEY", tools[0].Spec.Security.CredentialRef)

	// Verify file was saved
	toolPath := filepath.Join(tempDir, "finance", "gettest.json")
	_, err = os.Stat(toolPath)
	assert.NoError(t, err)
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

	toolPath := filepath.Join(tempDir, "hr", "simple-test.json")
	_, err = os.Stat(toolPath)
	assert.NoError(t, err)
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
