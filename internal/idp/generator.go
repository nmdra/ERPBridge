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
			Annotations: annotationsForHTTPMethod(method),
			InputSchema: mcp.InputSchema{
				Type:       generatedObjectType,
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
					Annotations: annotationsForHTTPMethod(method),
					InputSchema: mcp.InputSchema{
						Type:       generatedObjectType,
						Properties: make(map[string]mcp.Property),
					},
					Execution: mcp.Execution{
						Type:     "http",
						Method:   strings.ToUpper(method),
						Endpoint: baseURL + path,
					},
					Cache: defaultCache(method),
					Security: mcp.Security{
						AuthType:         api.AuthType,
						CredentialRef:    credentialRef,
						CredentialSource: api.CredentialSource,
					},
				},
			}

			parameters, err := mergeParameters(pathItem.Parameters, op.Parameters)
			if err != nil {
				return nil, fmt.Errorf("operation %q path %q: merge parameters: %w", toolName, path, err)
			}
			arguments := make([]generatedArgument, 0, len(parameters))
			for _, paramRef := range parameters {
				param := paramRef.Value
				location := strings.ToLower(strings.TrimSpace(param.In))
				if location != "path" && location != "query" && location != "header" {
					return nil, fmt.Errorf("operation %q path %q: unsupported parameter location %q", toolName, path, param.In)
				}
				if location == "header" && protectedGeneratedHeader(param.Name) {
					return nil, fmt.Errorf("operation %q path %q: protected header %q cannot be generated", toolName, path, param.Name)
				}
				if param.Schema == nil || param.Schema.Value == nil {
					return nil, fmt.Errorf("operation %q path %q: parameter %q has no schema", toolName, path, param.Name)
				}
				property, propertyErr := schemaProperty(doc, param.Schema.Value, "parameter "+param.Name)
				if propertyErr != nil {
					return nil, fmt.Errorf("operation %q path %q: %w", toolName, path, propertyErr)
				}
				if param.Description != "" {
					property.Description = param.Description
				}
				arguments = append(arguments, generatedArgument{
					sourceName: param.Name,
					erpName:    param.Name,
					location:   location,
					property:   property,
					required:   param.Required,
				})
			}

			bodyArgumentSource := ""
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				content := jsonSchemaContent(op.RequestBody.Value.Content)
				if content != nil && content.Schema != nil && content.Schema.Value != nil {
					bodySchema, bodyErr := schemaProperty(doc, content.Schema.Value, "request body")
					if bodyErr != nil {
						return nil, fmt.Errorf("operation %q path %q: %w", toolName, path, bodyErr)
					}
					if bodySchema.Type == generatedObjectType || len(bodySchema.Properties) > 0 {
						required := make(map[string]struct{}, len(bodySchema.Required))
						for _, name := range bodySchema.Required {
							required[name] = struct{}{}
						}
						names := make([]string, 0, len(bodySchema.Properties))
						for name := range bodySchema.Properties {
							names = append(names, name)
						}
						sort.Strings(names)
						for _, name := range names {
							_, isRequired := required[name]
							arguments = append(arguments, generatedArgument{
								sourceName: name,
								erpName:    name,
								location:   generatedBodyLocation,
								property:   bodySchema.Properties[name],
								required:   isRequired,
							})
						}
					} else {
						bodyArgumentSource = generatedBodyLocation
						arguments = append(arguments, generatedArgument{
							sourceName: bodyArgumentSource,
							erpName:    bodyArgumentSource,
							location:   generatedBodyLocation,
							property:   bodySchema,
							required:   op.RequestBody.Value.Required,
						})
					}
				}
			}

			arguments, err = finalizeGeneratedArguments(arguments)
			if err != nil {
				return nil, fmt.Errorf("operation %q path %q: %w", toolName, path, err)
			}
			for _, argument := range arguments {
				tool.Spec.InputSchema.Properties[argument.finalName] = argument.property
				if argument.required {
					tool.Spec.InputSchema.Required = append(tool.Spec.InputSchema.Required, argument.finalName)
				}
				if tool.Spec.Execution.Mapping == nil {
					tool.Spec.Execution.Mapping = make(map[string]string)
				}
				if tool.Spec.Execution.ParameterLocations == nil {
					tool.Spec.Execution.ParameterLocations = make(map[string]string)
				}
				tool.Spec.Execution.Mapping[argument.finalName] = argument.erpName
				tool.Spec.Execution.ParameterLocations[argument.finalName] = argument.location
				if argument.sourceName == bodyArgumentSource {
					tool.Spec.Execution.BodyArgument = argument.finalName
				}
			}
			sort.Strings(tool.Spec.InputSchema.Required)

			responsePath, outputSchema, responseErr := responseSchemaForOperation(doc, op, toolName, path)
			if responseErr != nil {
				return nil, responseErr
			}
			tool.Spec.Execution.ResponsePath = responsePath
			tool.Spec.OutputSchema = outputSchema

			g.log.Info("tool generated from OpenAPI", slog.String("tool_name", toolName))
			tools = append(tools, tool)
		}
	}

	return tools, nil
}

const generatedReadCacheTTLSeconds = 300

func annotationsForHTTPMethod(method string) *mcp.ToolAnnotations {
	annotations := &mcp.ToolAnnotations{OpenWorldHint: new(true)}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		annotations.ReadOnlyHint = new(true)
		annotations.DestructiveHint = new(false)
		annotations.IdempotentHint = new(true)
	case http.MethodPut, http.MethodDelete:
		annotations.ReadOnlyHint = new(false)
		annotations.DestructiveHint = new(true)
		annotations.IdempotentHint = new(true)
	case http.MethodPost, http.MethodPatch:
		annotations.ReadOnlyHint = new(false)
		annotations.DestructiveHint = new(true)
		annotations.IdempotentHint = new(false)
	}
	return annotations
}

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

const (
	generatedObjectType   = "object"
	generatedBodyLocation = "body"
)

type generatedArgument struct {
	sourceName string
	erpName    string
	location   string
	property   mcp.Property
	required   bool
	finalName  string
}

func mergeParameters(pathParameters, operationParameters openapi3.Parameters) (openapi3.Parameters, error) {
	merged := make(map[string]*openapi3.ParameterRef, len(pathParameters)+len(operationParameters))
	for _, ref := range append(pathParameters, operationParameters...) {
		if ref == nil {
			return nil, fmt.Errorf("parameter reference is empty")
		}
		if ref.Value == nil {
			return nil, fmt.Errorf("unresolved parameter reference %q", ref.Ref)
		}
		key := strings.ToLower(strings.TrimSpace(ref.Value.In)) + "\x00" + ref.Value.Name
		merged[key] = ref
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(openapi3.Parameters, 0, len(keys))
	for _, key := range keys {
		result = append(result, merged[key])
	}
	return result, nil
}

func jsonSchemaContent(content openapi3.Content) *openapi3.MediaType {
	if media := content.Get("application/json"); media != nil {
		return media
	}
	keys := make([]string, 0, len(content))
	for key := range content {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.HasSuffix(strings.ToLower(key), "+json") {
			return content[key]
		}
	}
	return nil
}

func schemaProperty(doc *openapi3.T, schema *openapi3.Schema, location string) (mcp.Property, error) {
	resolved, err := dereferenceSchemaAt(doc, schema, location)
	if err != nil {
		return mcp.Property{}, err
	}
	return propertyFromSchemaValue(resolved, location)
}

func dereferenceSchemaAt(doc *openapi3.T, schema *openapi3.Schema, location string) (any, error) {
	resolved, err := dereferenceSchema(doc, schema)
	if err != nil {
		return nil, fmt.Errorf("dereference %s schema: %w", location, err)
	}
	return resolved, nil
}

func propertyFromSchemaValue(value any, location string) (mcp.Property, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return mcp.Property{}, fmt.Errorf("%s schema is not an object", location)
	}
	property := mcp.Property{Type: "string"}
	if rawType, ok := object["type"].(string); ok && rawType != "" {
		property.Type = rawType
	}
	if description, ok := object["description"].(string); ok {
		property.Description = description
	}
	if defaultValue, ok := object["default"]; ok {
		property.Default = defaultValue
	}
	if enumValues, ok := object["enum"].([]any); ok {
		for _, enumValue := range enumValues {
			property.Enum = append(property.Enum, fmt.Sprint(enumValue))
		}
	}
	if required, ok := object["required"].([]any); ok {
		for _, name := range required {
			if name, ok := name.(string); ok {
				property.Required = append(property.Required, name)
			}
		}
		sort.Strings(property.Required)
	}
	if properties, ok := object["properties"].(map[string]any); ok {
		property.Properties = make(map[string]mcp.Property, len(properties))
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child, err := propertyFromSchemaValue(properties[name], location+"."+name)
			if err != nil {
				return mcp.Property{}, err
			}
			property.Properties[name] = child
		}
	}
	if items, ok := object["items"]; ok {
		child, err := propertyFromSchemaValue(items, location+"[]")
		if err != nil {
			return mcp.Property{}, err
		}
		property.Items = &child
	}
	return property, nil
}

func protectedGeneratedHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "proxy-authorization", "cookie", "host", "connection", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade", "content-length", "content-type":
		return true
	default:
		return false
	}
}

func finalizeGeneratedArguments(arguments []generatedArgument) ([]generatedArgument, error) {
	seenLocations := make(map[string]string, len(arguments))
	sourceCounts := make(map[string]int, len(arguments))
	for _, argument := range arguments {
		key := argument.location + "\x00" + argument.erpName
		if previous, exists := seenLocations[key]; exists {
			return nil, fmt.Errorf("generated parameter collision for ERP name %q in %s between %q and %q", argument.erpName, argument.location, previous, argument.sourceName)
		}
		seenLocations[key] = argument.sourceName
		sourceCounts[argument.sourceName]++
	}
	for index := range arguments {
		argument := &arguments[index]
		if sourceCounts[argument.sourceName] > 1 {
			argument.finalName = argument.sourceName + "__" + argument.location
		} else {
			argument.finalName = argument.sourceName
		}
	}
	order := make([]int, len(arguments))
	for index := range arguments {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := arguments[order[i]], arguments[order[j]]
		leftExact := left.finalName == left.sourceName
		rightExact := right.finalName == right.sourceName
		if leftExact != rightExact {
			return leftExact
		}
		if left.finalName != right.finalName {
			return left.finalName < right.finalName
		}
		if left.location != right.location {
			return left.location < right.location
		}
		return left.erpName < right.erpName
	})
	used := make(map[string]struct{}, len(arguments))
	for _, index := range order {
		argument := &arguments[index]
		base := argument.finalName
		candidate := base
		for suffix := 2; ; suffix++ {
			if _, exists := used[candidate]; !exists {
				break
			}
			candidate = fmt.Sprintf("%s__%d", base, suffix)
		}
		argument.finalName = candidate
		used[candidate] = struct{}{}
	}
	return arguments, nil
}

func responseSchemaForOperation(doc *openapi3.T, op *openapi3.Operation, toolName, path string) (string, *any, error) {
	var responseRef *openapi3.ResponseRef
	if op.Responses != nil {
		responseRef = op.Responses.Status(200)
		if responseRef == nil {
			responseRef = op.Responses.Status(201)
		}
	}
	if responseRef == nil || responseRef.Value == nil {
		return "", nil, nil
	}
	media := jsonSchemaContent(responseRef.Value.Content)
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		return "", nil, nil
	}
	resolved, err := dereferenceSchemaAt(doc, media.Schema.Value, fmt.Sprintf("output for operation %q path %q", toolName, path))
	if err != nil {
		return "", nil, err
	}
	output := resolved
	responsePath := ""
	if object, ok := resolved.(map[string]any); ok {
		if object["type"] == generatedObjectType {
			if properties, ok := object["properties"].(map[string]any); ok {
				if data, exists := properties["data"]; exists {
					responsePath = "data"
					output = data
				}
			}
		}
	}
	return responsePath, &output, nil
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
