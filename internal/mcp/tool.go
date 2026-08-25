package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaTypeObject = "object"
	schemaTypeString = "string"
	schemaTypeNumber = "number"
)

// Tool represents a versioned, protocol-compliant MCP tool resource.
type Tool struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       ToolSpec `json:"spec"`

	// Handler is an optional native Go function to handle the tool call.
	Handler func(ctx context.Context, args map[string]any) (*ToolResult, error) `json:"-"`
}

// Metadata contains identity and lifecycle information for a tool.
type Metadata struct {
	Name     string `json:"name"`
	Version  string `json:"version"` // SemVer
	Module   string `json:"module"`
	Status   string `json:"status,omitempty"`   // ready, degraded
	IsActive bool   `json:"isActive,omitempty"` // for soft-delete/visibility
}

// ToolSpec defines the behavior, interface, and execution details of a tool.
type ToolSpec struct {
	Description  Description   `json:"description"`
	InputSchema  InputSchema   `json:"inputSchema"`
	OutputSchema *any          `json:"outputSchema,omitempty"`
	Execution    Execution     `json:"execution"`
	Cache        *cache.Config `json:"cache,omitempty"`
	Security     Security      `json:"security"`
	Routing      *Routing      `json:"routing,omitempty"`
	Lifecycle    *Lifecycle    `json:"lifecycle,omitempty"`
}

// Description provides rich semantic information to the LLM.
type Description struct {
	Short        string   `json:"short"`
	WhenToUse    []string `json:"whenToUse"`
	WhenNotToUse []string `json:"whenNotToUse"`
	Examples     []string `json:"examples"`
}

// InputSchema defines the structure of arguments required by a tool.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single field in a tool's input schema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// Execution defines how the tool call is mapped to an ERP API.
type Execution struct {
	Type         string            `json:"type"` // "http"
	Method       string            `json:"method"`
	Endpoint     string            `json:"endpoint"`
	Mapping      map[string]string `json:"mapping,omitempty"`      // Maps LLM arg name -> ERP arg name
	ResponsePath string            `json:"responsePath,omitempty"` // JSONPath to unwrap response
}

// Security defines the authentication requirements for the tool.
type Security struct {
	AuthType      string   `json:"authType"`      // api-key, basic, bearer
	CredentialRef string   `json:"credentialRef"` // Env var name or vault key
	AllowedRoles  []string `json:"allowedRoles,omitempty"`
}

// Routing provides metadata to improve LLM tool selection accuracy.
type Routing struct {
	Priority    float64  `json:"priority"`
	Signals     []string `json:"signals"`
	AntiSignals []string `json:"antiSignals"`
}

// Lifecycle defines the support status of a specific tool version.
type Lifecycle struct {
	Status       string `json:"status"` // stable, deprecated, sunset
	DeprecatedAt string `json:"deprecatedAt,omitempty"`
	SunsetAt     string `json:"sunsetAt,omitempty"`
	Replacement  string `json:"replacement,omitempty"`
}

// ToolCallRequest represents an incoming request from an MCP client to invoke a tool.
type ToolCallRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResult encapsulates the outcome of a tool invocation.
type ToolResult struct {
	// Result contains the successful response data from the ERP.
	Result any `json:"result"`
	// Error contains details about a failed invocation.
	Error any `json:"error,omitempty"`
	// IsError indicates if the invocation failed.
	IsError bool `json:"isError,omitempty"`
}

// ERPConnector defines the interface for executing calls to external ERP systems.
type ERPConnector interface {
	Call(ctx context.Context, ep connector.EndpointConfig, queryParams url.Values, body io.Reader) (*http.Response, error)
}

// Execute performs the actual tool invocation by calling either a native handler or the underlying ERP API.
func (t *Tool) Execute(ctx context.Context, args map[string]any, conn ERPConnector) (*ToolResult, error) {
	if t.Handler != nil {
		return t.Handler(ctx, args)
	}

	if t.Spec.Execution.Endpoint == "" {
		return nil, fmt.Errorf("tool %s has no endpoint configuration", t.Metadata.Name)
	}

	// Apply argument mapping (LLM arg name -> ERP arg name)
	erpArgs := make(map[string]any)
	for k, v := range args {
		if mappedKey, ok := t.Spec.Execution.Mapping[k]; ok {
			erpArgs[mappedKey] = v
		} else {
			erpArgs[k] = v
		}
	}

	fullURL := t.Spec.Execution.Endpoint

	// Path parameter substitution
	for k, v := range erpArgs {
		placeholder := "{" + k + "}"
		if strings.Contains(fullURL, placeholder) {
			fullURL = strings.ReplaceAll(fullURL, placeholder, fmt.Sprintf("%v", v))
			delete(erpArgs, k) // Remove from args so it's not sent as query/body param
		}
	}

	queryParams := url.Values{}
	var body io.Reader

	if t.Spec.Execution.Method == http.MethodGet {
		for k, v := range erpArgs {
			queryParams.Set(k, fmt.Sprintf("%v", v))
		}
	} else if len(erpArgs) > 0 {
		data, err := json.Marshal(erpArgs)
		if err != nil {
			return nil, fmt.Errorf("marshal arguments: %w", err)
		}
		body = strings.NewReader(string(data))
	}

	envBaseURL := os.Getenv("ERP_BASE_URL")

	if envBaseURL != "" {
		u, err := url.Parse(fullURL)
		if err == nil {
			if u.IsAbs() {
				// If it's an absolute URL pointing to localhost, override it with ERP_BASE_URL
				if strings.HasPrefix(u.Host, "localhost") || strings.HasPrefix(u.Host, "127.0.0.1") || strings.HasPrefix(u.Host, "[::1]") {
					base, err := url.Parse(envBaseURL)
					if err == nil {
						u.Scheme = base.Scheme
						u.Host = base.Host
						fullURL = u.String()
					}
				}
			} else {
				// If it's a relative path, simply prepend the base URL
				fullURL = strings.TrimSuffix(envBaseURL, "/") + "/" + strings.TrimPrefix(fullURL, "/")
			}
		}
	} else if !strings.HasPrefix(fullURL, "http") {
		// Fallback for relative paths if no environment variable is set
		fullURL = "http://localhost:8081" + "/" + strings.TrimPrefix(fullURL, "/")
	}

	// Resolve CredentialRef from environment variables. A configured but missing
	// reference must fail closed rather than being sent as a literal credential.
	cred, err := resolveCredential(t.Spec.Security.CredentialRef)
	if err != nil {
		return nil, err
	}

	ep := connector.EndpointConfig{
		Method:  t.Spec.Execution.Method,
		Path:    fullURL,
		BaseURL: "",
		Auth: connector.AuthConfig{
			Type: t.Spec.Security.AuthType,
			Key:  cred,
		},
	}

	resp, err := conn.Call(ctx, ep, queryParams, body)
	if err != nil {
		return nil, fmt.Errorf("erp call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var resultData any
	if err := json.NewDecoder(resp.Body).Decode(&resultData); err != nil {
		return nil, fmt.Errorf("decode erp response: %w", err)
	}

	// Unwrap response based on ResponsePath.
	if t.Spec.Execution.ResponsePath != "" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resultData, err = resolveResponsePath(resultData, t.Spec.Execution.ResponsePath)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := t.ValidateResult(resultData); err != nil {
			return &ToolResult{
				Result:  resultData,
				Error:   fmt.Sprintf("response validation failed: %v", err),
				IsError: true,
			}, nil
		}
	}

	return &ToolResult{
		Result:  resultData,
		IsError: resp.StatusCode >= 400,
	}, nil
}

type responsePathToken struct {
	key   string
	index *int
}

func resolveResponsePath(root any, responsePath string) (any, error) {
	tokens, err := parseResponsePath(responsePath)
	if err != nil {
		return nil, fmt.Errorf("invalid response path %q: %w", responsePath, err)
	}

	current := root
	for _, token := range tokens {
		if token.key != "" {
			object, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("response path %q not found: expected object for %q", responsePath, token.key)
			}
			var exists bool
			current, exists = object[token.key]
			if !exists {
				return nil, fmt.Errorf("response path %q not found", responsePath)
			}
		}
		if token.index != nil {
			items, ok := current.([]any)
			if !ok || *token.index < 0 || *token.index >= len(items) {
				return nil, fmt.Errorf("response path %q not found: array index %d is out of bounds", responsePath, *token.index)
			}
			current = items[*token.index]
		}
	}
	return current, nil
}

func parseResponsePath(responsePath string) ([]responsePathToken, error) {
	if responsePath == "" {
		return nil, fmt.Errorf("path is empty")
	}

	var tokens []responsePathToken
	for position := 0; position < len(responsePath); {
		if responsePath[position] == '.' {
			return nil, fmt.Errorf("empty object segment")
		}
		if responsePath[position] != '[' {
			start := position
			for position < len(responsePath) && responsePath[position] != '.' && responsePath[position] != '[' {
				position++
			}
			if start == position {
				return nil, fmt.Errorf("empty object segment")
			}
			tokens = append(tokens, responsePathToken{key: responsePath[start:position]})
		}

		for position < len(responsePath) && responsePath[position] == '[' {
			end := strings.IndexByte(responsePath[position+1:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated array index")
			}
			end += position + 1
			index, err := strconv.Atoi(responsePath[position+1 : end])
			if err != nil || index < 0 {
				return nil, fmt.Errorf("invalid array index")
			}
			tokens = append(tokens, responsePathToken{index: &index})
			position = end + 1
		}

		if position < len(responsePath) {
			if responsePath[position] != '.' {
				return nil, fmt.Errorf("unexpected character %q", responsePath[position])
			}
			position++
			if position == len(responsePath) {
				return nil, fmt.Errorf("path ends with a separator")
			}
		}
	}
	return tokens, nil
}

func resolveCredential(ref string) (string, error) {
	return credentials.Resolve(ref)
}

// ValidateResult checks a normalized successful result against the tool's
// declared output schema. A tool without an output schema accepts any JSON value.
func (t *Tool) ValidateResult(data any) error {
	if t == nil || t.Spec.OutputSchema == nil {
		return nil
	}
	return validateResponse(data, t.Spec.OutputSchema)
}

func validateResponse(data any, schema any) error {
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	var schemaDocument any
	if err := json.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schemaDocument); err != nil {
		return fmt.Errorf("add resource: %w", err)
	}
	js, err := c.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	return js.Validate(data)
}
