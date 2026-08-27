// Package idp provides API registration and OpenAPI-to-MCP tool schema generation.
package idp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

// Generator transforms API definitions and OpenAPI specs into declarative MCP tools.
type Generator struct {
	SchemasDir string
	log        *slog.Logger
}

// NewGenerator creates a new Generator instance with the given schema directory.
func NewGenerator(schemasDir string, rootLog *slog.Logger) *Generator {
	if schemasDir == "" {
		schemasDir = "schemas"
	}
	return &Generator{
		SchemasDir: schemasDir,
		log:        logger.Component(rootLog, "idp"),
	}
}

// Generate creates a basic MCP Tool from a registered API definition.
func (g *Generator) Generate(api API) (*mcp.Tool, error) {
	credentialRef, err := credentialRefForAPI(api)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(api.Method))
	name := api.Name
	// Simple intent-based naming heuristic if not already provided.
	if after, ok := strings.CutPrefix(strings.ToLower(name), "get-"); ok {
		name = "get_" + after
	}

	tool := &mcp.Tool{
		APIVersion: "erpbridge.io/v1",
		Kind:       "MCPTool",
		Metadata: mcp.Metadata{
			Name:    name,
			Version: "1.0.0",
			Module:  api.Module,
		},
		Spec: mcp.ToolSpec{
			Description: intentDescription(api.Description),
			InputSchema: mcp.InputSchema{
				Type:       "object",
				Properties: make(map[string]mcp.Property),
			},
			Execution: mcp.Execution{
				Type:     "http",
				Method:   method,
				Endpoint: api.URL,
			},
			Cache: defaultCache(method),
			Security: mcp.Security{
				AuthType:         api.AuthType,
				CredentialRef:    credentialRef,
				CredentialSource: api.CredentialSource,
			},
		},
	}

	if method == http.MethodGet || method == http.MethodHead {
		tool.Spec.InputSchema.Properties["page"] = mcp.Property{
			Type:        "integer",
			Description: "Page number for pagination",
			Default:     1,
		}
	}

	g.log.Info("tool generated", slog.String("tool_name", tool.Metadata.Name))
	return tool, nil
}

// GenerateFromOpenAPI parses an OpenAPI 3.0 specification from a URL or file path and generates MCP tools.
func (g *Generator) GenerateFromOpenAPI(ctx context.Context, api API, openapiURL string) ([]*mcp.Tool, error) {
	credentialRef, err := credentialRefForAPI(api)
	if err != nil {
		return nil, err
	}
	loader := openapi3.NewLoader()
	var doc *openapi3.T

	if strings.HasPrefix(openapiURL, "http") {
		u, err := url.Parse(openapiURL)
		if err != nil {
			return nil, fmt.Errorf("invalid openapi url: %w", err)
		}
		doc, err = loader.LoadFromURI(u)
		if err != nil {
			// Fallback: manually fetch if LoadFromURI fails
			req, httpErr := http.NewRequestWithContext(ctx, http.MethodGet, openapiURL, nil)
			if httpErr != nil {
				return nil, fmt.Errorf("failed to create request: %w", httpErr)
			}
			resp, httpErr := http.DefaultClient.Do(req)
			if httpErr != nil {
				return nil, fmt.Errorf("failed to fetch openapi spec: %w", httpErr)
			}
			defer func() { _ = resp.Body.Close() }()
			doc, err = loader.LoadFromIoReader(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to parse fetched openapi spec: %w", err)
			}
		}
	} else {
		doc, err = loader.LoadFromFile(openapiURL)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("invalid openapi spec: %w", err)
	}

	var tools []*mcp.Tool
	seenNames := make(map[string]string)
	paths := make([]string, 0, len(doc.Paths.Map()))
	for path := range doc.Paths.Map() {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pathItem := doc.Paths.Map()[path]
		if pathItem == nil {
			continue
		}
		methods := make([]string, 0, len(pathItem.Operations()))
		for method := range pathItem.Operations() {
			methods = append(methods, method)
		}
		sort.Slice(methods, func(i, j int) bool {
			iRank, jRank := methodRank(methods[i]), methodRank(methods[j])
			if iRank != jRank {
				return iRank < jRank
			}
			return methods[i] < methods[j]
		})
		for _, method := range methods {
			op := pathItem.Operations()[method]
			if op == nil {
				continue
			}
			toolName := op.OperationID
			if toolName == "" {
				// Sanitize path for tool name
				safePath := strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")
				safePath = strings.ReplaceAll(safePath, "-", "_")

				prefix := strings.ToLower(method)
				switch prefix {
				case "post":
					prefix = "create"
				case "get":
					if strings.Contains(path, "{") {
						prefix = "get"
					} else {
						prefix = "list"
					}
				}

				// Remove 'resource_' from path if it exists to keep it semantic
				safePath = strings.TrimPrefix(safePath, "resource_")

				// Handle Pluralization for 'list' operations
				if prefix == "list" {
					if !strings.HasSuffix(safePath, "s") {
						// Simple pluralization for known entities
						entities := map[string]string{
							"bin":               "bins",
							"department":        "departments",
							"employee":          "employees",
							"item":              "items",
							"journal_entry":     "journal_entries",
							"leave_application": "leave_applications",
							"payment_entry":     "payment_entries",
							"purchase_invoice":  "purchase_invoices",
							"purchase_order":    "purchase_orders",
							"salary_slip":       "salary_slips",
						}
						if p, ok := entities[safePath]; ok {
							safePath = p
						} else if before, ok0 := strings.CutSuffix(safePath, "y"); ok0 {
							safePath = before + "ies"
						} else {
							safePath += "s"
						}
					}
				}

				toolName = prefix + "_" + safePath
			}

			toolName = sanitizeOperationName(toolName)
			if toolName == "" {
				return nil, fmt.Errorf("generated tool name is empty for %s %s", strings.ToUpper(method), path)
			}
			source := fmt.Sprintf("%s %s", strings.ToUpper(method), path)
			if previous, exists := seenNames[toolName]; exists {
				return nil, fmt.Errorf("generated tool name collision %q between %s and %s", toolName, previous, source)
			}
			seenNames[toolName] = source

			baseURL := ""
			if len(doc.Servers) > 0 {
				baseURL = doc.Servers[0].URL
			}

			short := strings.TrimSpace(op.Summary)
			if short == "" {
				short = strings.TrimSpace(op.Description)
			}
			if short == "" {
				short = fmt.Sprintf("%s %s", strings.ToUpper(method), path)
			}
			tool := &mcp.Tool{
				APIVersion: "erpbridge.io/v1",
				Kind:       "MCPTool",
				Metadata: mcp.Metadata{
					Name:    toolName,
					Version: "1.0.0",
					Module:  api.Module,
				},
				Spec: mcp.ToolSpec{
					Description: intentDescription(short),
					InputSchema: mcp.InputSchema{
						Type:       "object",
						Properties: make(map[string]mcp.Property),
					},
					Execution: mcp.Execution{
						Type:         "http",
						Method:       strings.ToUpper(method),
						Endpoint:     baseURL + path,
						ResponsePath: "data",
					},
					Cache: defaultCache(method),
					Security: mcp.Security{
						AuthType:         api.AuthType,
						CredentialRef:    credentialRef,
						CredentialSource: api.CredentialSource,
					},
				},
			}

			// Map parameters
			for _, paramRef := range op.Parameters {
				if paramRef == nil || paramRef.Value == nil {
					continue
				}
				param := paramRef.Value
				if param.Schema == nil || param.Schema.Value == nil {
					continue
				}
				p := mcp.Property{
					Type:        "string", // Default
					Description: param.Description,
				}
				if len(param.Schema.Value.Type.Slice()) > 0 {
					p.Type = param.Schema.Value.Type.Slice()[0]
				}
				if param.Required {
					tool.Spec.InputSchema.Required = append(tool.Spec.InputSchema.Required, param.Name)
				}
				tool.Spec.InputSchema.Properties[param.Name] = p
			}

			// Map Request Body (for POST/PATCH)
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				content := op.RequestBody.Value.Content.Get("application/json")
				if content != nil && content.Schema != nil && content.Schema.Value != nil {
					schema := content.Schema.Value
					for propName, propRef := range schema.Properties {
						if propRef == nil || propRef.Value == nil {
							continue
						}
						prop := propRef.Value
						p := mcp.Property{
							Type:        "string",
							Description: prop.Description,
						}
						if len(prop.Type.Slice()) > 0 {
							p.Type = prop.Type.Slice()[0]
						}
						tool.Spec.InputSchema.Properties[propName] = p
					}
					tool.Spec.InputSchema.Required = append(tool.Spec.InputSchema.Required, schema.Required...)
				}
			}

			// Task 6: Response Validation (infer output schema)
			resp200 := op.Responses.Status(200)
			if resp200 == nil {
				resp200 = op.Responses.Status(201)
			}
			if resp200 != nil && resp200.Value != nil && resp200.Value.Content.Get("application/json") != nil {
				schemaRef := resp200.Value.Content.Get("application/json").Schema
				if schemaRef != nil && schemaRef.Value != nil {
					outputSchema, err := dereferenceSchema(doc, schemaRef.Value)
					if err != nil {
						return nil, fmt.Errorf("operation %q path %q: dereference output schema: %w", toolName, path, err)
					}
					tool.Spec.OutputSchema = &outputSchema
				}
			}

			g.log.Info("tool generated from OpenAPI", slog.String("tool_name", toolName))
			tools = append(tools, tool)
		}
	}

	return tools, nil
}

const generatedReadCacheTTLSeconds = 300

func defaultCache(method string) *cache.Config {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == http.MethodGet || method == http.MethodHead {
		return &cache.Config{Enabled: true, TTLSeconds: generatedReadCacheTTLSeconds, IsReadOnly: true, FlushOn: []string{}}
	}
	return &cache.Config{Enabled: false, TTLSeconds: 0, IsReadOnly: false, FlushOn: []string{}}
}

func intentDescription(short string) mcp.Description {
	short = strings.TrimSpace(short)
	description := mcp.Description{Short: short}
	if short == "" {
		return description
	}
	// These phrases are deliberately derived only from the operation summary or
	// description. They give agents routing evidence without inventing domain
	// details that are not present in the OpenAPI contract.
	description.WhenToUse = []string{"Use when the user asks for: " + short}
	description.Examples = []string{"I need help with: " + short}
	return description
}

func methodRank(method string) int {
	for rank, candidate := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodTrace,
	} {
		if strings.EqualFold(method, candidate) {
			return rank
		}
	}
	return 100
}

func sanitizeOperationName(name string) string {
	var sanitized strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sanitized.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && sanitized.Len() > 0 {
			sanitized.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(sanitized.String(), "_")
}

func credentialRefForAPI(api API) (string, error) {
	if err := credentials.ValidateCredentialSource(api.CredentialSource); err != nil {
		return "", err
	}
	if credentials.IsFileBacked(api.CredentialSource) && api.CredentialRef == "" {
		return "", fmt.Errorf("API %q requires a credential reference for file credentials", api.Name)
	}
	if api.AuthType == "" {
		return "", nil
	}
	if api.CredentialRef == "" {
		return "", fmt.Errorf("API %q requires a credential reference before generation", api.Name)
	}
	if err := credentials.ValidateReference(api.CredentialRef); err != nil {
		return "", err
	}
	return api.CredentialRef, nil
}

func dereferenceSchema(doc *openapi3.T, schema *openapi3.Schema) (any, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	refs := make(map[string]*openapi3.Schema)
	if doc.Components != nil {
		for ref, schemaRef := range doc.Components.Schemas {
			if schemaRef != nil && schemaRef.Value != nil {
				refs["#/components/schemas/"+ref] = schemaRef.Value
			}
		}
	}
	return resolveSchemaValue(value, refs, "output")
}

func resolveSchemaValue(value any, refs map[string]*openapi3.Schema, location string) (any, error) {
	switch current := value.(type) {
	case map[string]any:
		if ref, ok := current["$ref"].(string); ok {
			schema, exists := refs[ref]
			if !exists || schema == nil {
				return nil, fmt.Errorf("unresolved reference %q at %s", ref, location)
			}
			raw, err := json.Marshal(schema)
			if err != nil {
				return nil, fmt.Errorf("marshal reference %q: %w", ref, err)
			}
			var resolved any
			if err := json.Unmarshal(raw, &resolved); err != nil {
				return nil, fmt.Errorf("decode reference %q: %w", ref, err)
			}
			resolved, err = resolveSchemaValue(resolved, refs, location+"/"+ref)
			if err != nil {
				return nil, err
			}
			resolvedMap, ok := resolved.(map[string]any)
			if !ok {
				return resolved, nil
			}
			for key, child := range current {
				if key != "$ref" {
					resolvedMap[key] = child
				}
			}
			current = resolvedMap
		}
		for key, child := range current {
			resolved, err := resolveSchemaValue(child, refs, location+"/"+key)
			if err != nil {
				return nil, err
			}
			current[key] = resolved
		}
		return current, nil
	case []any:
		for i, child := range current {
			resolved, err := resolveSchemaValue(child, refs, fmt.Sprintf("%s[%d]", location, i))
			if err != nil {
				return nil, err
			}
			current[i] = resolved
		}
		return current, nil
	default:
		return value, nil
	}
}

// Save writes the MCP tool definition to disk as formatted JSON.
func (g *Generator) Save(tool *mcp.Tool) error {
	dir := filepath.Join(g.SchemasDir, tool.Metadata.Module)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	path := filepath.Join(dir, tool.Metadata.Name+".json")
	data, err := json.MarshalIndent(tool, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
