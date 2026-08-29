package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/credentials"
	"github.com/nmdra/ERPBridge/internal/faults"
	"github.com/nmdra/ERPBridge/internal/metrics"
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
	Description  Description      `json:"description"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	RateLimit    *ToolRateLimit   `json:"rateLimit,omitempty"`
	Concurrency  *ToolConcurrency `json:"concurrency,omitempty"`
	InputSchema  InputSchema      `json:"inputSchema"`
	OutputSchema *any             `json:"outputSchema,omitempty"`
	Execution    Execution        `json:"execution"`
	Cache        *cache.Config    `json:"cache,omitempty"`
	Security     Security         `json:"security"`
	Routing      *Routing         `json:"routing,omitempty"`
	Lifecycle    *Lifecycle       `json:"lifecycle,omitempty"`
}

// Description provides rich semantic information to the LLM.
type Description struct {
	Short        string   `json:"short"`
	WhenToUse    []string `json:"whenToUse"`
	WhenNotToUse []string `json:"whenNotToUse"`
	Examples     []string `json:"examples"`
}

// ToolAnnotations describes the expected behavior of a tool for MCP clients.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ToolRateLimit optionally adds an independent token bucket for one tool.
type ToolRateLimit struct {
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	Burst             int     `json:"burst"`
}

// Validate checks that the optional per-tool rate limit can make progress.
func (r ToolRateLimit) Validate() error {
	return RateLimitConfig(r).Validate()
}

// ToolConcurrency optionally bounds simultaneous executions of one tool.
type ToolConcurrency struct {
	Limit        int  `json:"limit"`
	PerPrincipal bool `json:"perPrincipal,omitempty"`
}

// Validate checks that the optional concurrency limit is positive.
func (c ToolConcurrency) Validate() error {
	if c.Limit <= 0 {
		return errors.New("tool concurrency limit must be positive")
	}
	return nil
}

// InputSchema defines the structure of arguments required by a tool.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single field in a tool's input schema.
type Property struct {
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Enum        []string            `json:"enum,omitempty"`
	Default     any                 `json:"default,omitempty"`
	Properties  map[string]Property `json:"properties,omitempty"`
	Items       *Property           `json:"items,omitempty"`
	Required    []string            `json:"required,omitempty"`
}

// Execution defines how the tool call is mapped to an ERP API.
type Execution struct {
	Type               string            `json:"type"` // "http"
	Method             string            `json:"method"`
	Endpoint           string            `json:"endpoint"`
	Mapping            map[string]string `json:"mapping,omitempty"`            // Maps LLM arg name -> ERP arg name
	ParameterLocations map[string]string `json:"parameterLocations,omitempty"` // Maps LLM arg name -> path/query/header/body
	BodyArgument       string            `json:"bodyArgument,omitempty"`       // Complete primitive/array request-body argument
	ResponsePath       string            `json:"responsePath,omitempty"`       // JSONPath to unwrap response
}

// Security defines the authentication requirements for the tool.
type Security struct {
	AuthType         string                       `json:"authType"`      // api-key, basic, bearer
	CredentialRef    string                       `json:"credentialRef"` // Logical environment variable name
	CredentialSource credentials.CredentialSource `json:"credentialSource,omitempty"`
	DataClass        string                       `json:"dataClass,omitempty"`
	AllowedRoles     []string                     `json:"allowedRoles,omitempty"`
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

// ERPResponseConnector is an optional connector capability for raw-response
// processing. Its legacy Call method remains the default execution path.
type ERPResponseConnector interface {
	CallWithOptions(ctx context.Context, ep connector.EndpointConfig, queryParams url.Values, body io.Reader, options connector.CallOptions) (*http.Response, error)
}

// ERPResponse is a bounded response captured before JSON decoding.
type ERPResponse struct {
	Status      int
	ContentType string
	Body        []byte
	RetryAfter  time.Duration `json:"-"`
}

func (t *Tool) prepareERPCall(args map[string]any) (connector.EndpointConfig, url.Values, io.Reader, error) {
	if t.Spec.Execution.Endpoint == "" {
		return connector.EndpointConfig{}, nil, nil, fmt.Errorf("tool %s has no endpoint configuration", t.Metadata.Name)
	}

	method := strings.ToUpper(strings.TrimSpace(t.Spec.Execution.Method))
	fullURL := t.Spec.Execution.Endpoint
	queryParams := url.Values{}
	generatedHeaders := make(map[string]string)
	bodyFields := make(map[string]any)
	var completeBody any
	completeBodySet := false

	argumentNames := make([]string, 0, len(args))
	for name := range args {
		argumentNames = append(argumentNames, name)
	}
	sort.Strings(argumentNames)

	for _, name := range argumentNames {
		value := args[name]
		mappedName := name
		if mapped, ok := t.Spec.Execution.Mapping[name]; ok && mapped != "" {
			mappedName = mapped
		}
		location, hasLocation := t.Spec.Execution.ParameterLocations[name]
		location = strings.ToLower(strings.TrimSpace(location))
		if !hasLocation || location == "" {
			// Manifests written before parameterLocations retain their original
			// GET-query/non-GET-body behavior.
			placeholder := "{" + mappedName + "}"
			if strings.Contains(fullURL, placeholder) {
				fullURL = strings.ReplaceAll(fullURL, placeholder, url.PathEscape(fmt.Sprintf("%v", value)))
				continue
			}
			if method == http.MethodGet {
				queryParams.Set(mappedName, fmt.Sprintf("%v", value))
			} else {
				bodyFields[mappedName] = value
			}
			continue
		}

		switch location {
		case "path":
			placeholder := "{" + mappedName + "}"
			if !strings.Contains(fullURL, placeholder) {
				return connector.EndpointConfig{}, nil, nil, fmt.Errorf("path parameter %q is not present in endpoint", mappedName)
			}
			fullURL = strings.ReplaceAll(fullURL, placeholder, url.PathEscape(fmt.Sprintf("%v", value)))
		case "query":
			queryParams.Set(mappedName, fmt.Sprintf("%v", value))
		case "header":
			generatedHeaders[mappedName] = fmt.Sprintf("%v", value)
		case "body":
			if t.Spec.Execution.BodyArgument == name {
				completeBody = value
				completeBodySet = true
			} else {
				bodyFields[mappedName] = value
			}
		default:
			return connector.EndpointConfig{}, nil, nil, fmt.Errorf("unsupported parameter location %q", location)
		}
	}

	var body io.Reader
	if completeBodySet {
		data, err := json.Marshal(completeBody)
		if err != nil {
			return connector.EndpointConfig{}, nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = strings.NewReader(string(data))
	} else if len(bodyFields) > 0 {
		data, err := json.Marshal(bodyFields)
		if err != nil {
			return connector.EndpointConfig{}, nil, nil, fmt.Errorf("marshal arguments: %w", err)
		}
		body = strings.NewReader(string(data))
	}

	envBaseURL := os.Getenv("ERP_BASE_URL")
	if envBaseURL != "" {
		u, err := url.Parse(fullURL)
		if err == nil {
			if u.IsAbs() {
				if isLocalEndpoint(u) {
					base, err := url.Parse(envBaseURL)
					if err == nil {
						u.Scheme = base.Scheme
						u.Host = base.Host
						fullURL = u.String()
					}
				}
			} else {
				fullURL = strings.TrimSuffix(envBaseURL, "/") + "/" + strings.TrimPrefix(fullURL, "/")
			}
		}
	} else if !strings.HasPrefix(fullURL, "http") {
		fullURL = "http://localhost:8081" + "/" + strings.TrimPrefix(fullURL, "/")
	}

	cred, err := resolveCredential(t.Spec.Security.CredentialRef, t.Spec.Security.CredentialSource)
	if err != nil {
		return connector.EndpointConfig{}, nil, nil, err
	}
	return connector.EndpointConfig{
		Method:  method,
		Path:    fullURL,
		BaseURL: "",
		Headers: generatedHeaders,
		Auth: connector.AuthConfig{
			Type: t.Spec.Security.AuthType,
			Key:  cred,
		},
	}, queryParams, body, nil
}

func (t *Tool) callERPResponse(ctx context.Context, args map[string]any, conn ERPConnector, options connector.CallOptions) (*http.Response, error) {
	ep, queryParams, body, err := t.prepareERPCall(args)
	if err != nil {
		return nil, err
	}
	if options.PreserveErrorResponses {
		responseConnector, ok := conn.(ERPResponseConnector)
		if !ok {
			return nil, errors.New("ERP connector does not support raw response capture")
		}
		return responseConnector.CallWithOptions(ctx, ep, queryParams, body, options)
	}
	return conn.Call(ctx, ep, queryParams, body)
}

// CallERP captures an ERP response before JSON decoding for raw processing.
func (t *Tool) CallERP(ctx context.Context, args map[string]any, conn ERPConnector, options connector.CallOptions) (*ERPResponse, error) {
	resp, err := t.callERPResponse(ctx, args, conn, options)
	if err != nil {
		return nil, fmt.Errorf("erp call failed: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("erp response body is unavailable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPluginJSONBytes+1))
	if err != nil {
		return nil, errors.New("read ERP response")
	}
	if len(body) > MaxPluginJSONBytes {
		return nil, errors.New("ERP response exceeds maximum size")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if mediaType, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = strings.ToLower(mediaType)
	} else {
		contentType = "application/octet-stream"
	}
	retryAfter, _ := connector.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	return &ERPResponse{Status: resp.StatusCode, ContentType: contentType, Body: body, RetryAfter: retryAfter}, nil
}

// Execute performs the actual tool invocation by calling either a native handler or the underlying ERP API.
func (t *Tool) Execute(ctx context.Context, args map[string]any, conn ERPConnector) (*ToolResult, error) {
	if t.Handler != nil {
		return t.Handler(ctx, args)
	}
	options := connector.CallOptions{}
	if _, ok := conn.(ERPResponseConnector); ok {
		options.PreserveErrorResponses = true
	}
	resp, err := t.callERPResponse(ctx, args, conn, options)
	if err != nil {
		if failure, ok := faults.As(err); ok {
			recordDependencyFault(failure)
			return nil, failure
		}
		failure := faults.New(faults.KindInternal, "the tool could not prepare the ERP request; check server logs", false, 0, err)
		recordDependencyFault(failure)
		return nil, failure
	}
	if resp == nil {
		failure := faults.New(faults.KindDependencyUnavailable, "the ERP response was unavailable; retry later", true, 0, errors.New("nil ERP response"))
		recordDependencyFault(failure)
		return nil, failure
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := faultForHTTPResponse(resp)
		recordDependencyFault(failure)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return &ToolResult{Error: failure, IsError: true}, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 &&
		(strings.EqualFold(t.Spec.Execution.Method, http.MethodHead) || resp.StatusCode == http.StatusNoContent) {
		if resp.Body != nil {
			defer func() { _ = resp.Body.Close() }()
		}
		return &ToolResult{Result: nil}, nil
	}
	if resp.Body == nil {
		failure := faults.New(faults.KindDependencyUnavailable, "the ERP response body was unavailable; retry later", true, 0, errors.New("nil ERP response body"))
		recordDependencyFault(failure)
		return nil, failure
	}
	defer func() { _ = resp.Body.Close() }()

	var resultData any
	if err := json.NewDecoder(resp.Body).Decode(&resultData); err != nil {
		failure := faults.New(faults.KindDependencyUnavailable, "the ERP response was invalid; retry later", true, 0, err)
		recordDependencyFault(failure)
		return nil, failure
	}
	if t.Spec.Execution.ResponsePath != "" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resultData, err = resolveResponsePath(resultData, t.Spec.Execution.ResponsePath)
		if err != nil {
			failure := faults.New(faults.KindInternal, "the ERP response did not match the tool contract", false, 0, err)
			return nil, failure
		}
	}
	if err := t.ValidateResult(resultData); err != nil {
		return &ToolResult{
			Error:   faults.New(faults.KindInternal, "the ERP response did not match the tool contract", false, 0, err),
			IsError: true,
		}, nil
	}
	return &ToolResult{Result: resultData}, nil
}

func recordDependencyFault(err error) {
	fault, ok := faults.As(err)
	if !ok {
		return
	}
	metrics.DependencyErrorsTotal.WithLabelValues("erp", faultTypeName(fault.Kind)).Inc()
}

func faultForHTTPResponse(resp *http.Response) *faults.Error {
	retryAfter, _ := connector.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	return faultForHTTPStatus(resp.StatusCode, retryAfter)
}

func faultForHTTPStatus(status int, retryAfter time.Duration) *faults.Error {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return faults.New(faults.KindInvalidInput, "the ERP rejected the request; review the tool arguments", false, 0, nil)
	case http.StatusUnauthorized, http.StatusForbidden:
		return faults.New(faults.KindPermissionDenied, "the ERP denied this operation; verify access", false, 0, nil)
	case http.StatusNotFound:
		return faults.New(faults.KindNotFound, "the requested ERP resource was not found; check the arguments", false, 0, nil)
	case http.StatusConflict:
		return faults.New(faults.KindConflict, "the ERP operation conflicted with current state; review before retrying", false, 0, nil)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return faults.New(faults.KindDependencyTimeout, "the ERP service timed out; retry later", true, retryAfter, nil)
	case http.StatusTooManyRequests:
		message := "the ERP service is temporarily rate limited; retry later"
		if retryAfter > 0 {
			message = "the ERP service is temporarily rate limited; retry after " + retryAfterText(retryAfter)
		}
		return faults.New(faults.KindRateLimited, message, true, retryAfter, nil)
	default:
		if status >= 500 {
			return faults.New(faults.KindDependencyUnavailable, "the ERP service is temporarily unavailable; retry later", true, retryAfter, nil)
		}
		return faults.New(faults.KindInternal, "the ERP returned an unexpected response; check server logs", false, 0, nil)
	}
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

func isLocalEndpoint(u *url.URL) bool {
	if u == nil {
		return false
	}
	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func resolveCredential(ref string, source credentials.CredentialSource) (string, error) {
	return credentials.Resolve(ref, source)
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
