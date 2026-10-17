package climcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// SchemaVersion is the version of the adapter result envelope.
const SchemaVersion = 1

// Stable adapter result codes. These describe outcomes for the calling agent;
// they are not CLI exit codes or protocol error codes.
const (
	CodeInvalidArguments     = "invalid_arguments"
	CodePermissionDenied     = "permission_denied"
	CodeConfirmationMismatch = "confirmation_mismatch"
	CodeBusy                 = "busy"
	CodeContextChanged       = "context_changed"
	CodeNotFound             = "not_found"
	CodeAuthentication       = "authentication_required"
	CodeAuthorizationDenied  = "authorization_denied"
	CodeConflict             = "conflict"
	CodeUpstreamUnavailable  = "upstream_unavailable"
	CodeInvalidChildOutput   = "invalid_child_output"
	CodeOutputLimit          = "output_limit"
	CodeCancelled            = "cancelled"
	CodeTimeout              = "timeout"
	CodeOutcomeUnknown       = "outcome_unknown"
	CodeProbeFailed          = "probe_failed"
	CodeInternal             = "internal_error"
)

// Result is the adapter-owned envelope returned by every management tool.
type Result struct {
	SchemaVersion int    `json:"schemaVersion"`
	Operation     string `json:"operation"`
	Context       string `json:"context"`
	Success       bool   `json:"success"`
	Data          any    `json:"data,omitempty"`
	Error         *Error `json:"error,omitempty"`
}

// Error is a safe, adapter-owned failure description. It never carries child
// output, credentials, URLs, or upstream bodies.
type Error struct {
	Code       string        `json:"code"`
	Message    string        `json:"message"`
	Suggestion string        `json:"suggestion,omitempty"`
	Cause      string        `json:"cause,omitempty"`
	Details    *ProbeDetails `json:"details,omitempty"`
}

// ProbeDetails carries the typed outcome of a completed server-side API probe.
type ProbeDetails struct {
	APIName        string `json:"apiName,omitempty"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	ContentType    string `json:"contentType,omitempty"`
	LatencyMs      int64  `json:"latencyMs,omitempty"`
	ProbeSucceeded bool   `json:"probeSucceeded"`
}

// WriteData reports a completed write command. It reports command completion,
// never reconciliation or remote verification.
type WriteData struct {
	Target   string `json:"target"`
	Effect   string `json:"effect"`
	Verified bool   `json:"verified"`
	Replaced *bool  `json:"replaced,omitempty"`
}

// ContextSelectData reports a saved-default change. The running session keeps
// its pinned context.
type ContextSelectData struct {
	SavedDefault  string `json:"savedDefault"`
	ActiveContext string `json:"activeContext"`
	Verified      bool   `json:"verified"`
}

// ProbeData is the typed outcome of a completed server-side API test.
type ProbeData struct {
	APIName        string `json:"apiName"`
	HTTPStatus     int    `json:"httpStatus"`
	ContentType    string `json:"contentType,omitempty"`
	LatencyMs      int64  `json:"latencyMs"`
	ProbeSucceeded bool   `json:"probeSucceeded"`
}

// CacheFlushData reports the counters returned by a completed flush.
type CacheFlushData struct {
	Target   string `json:"target"`
	Deleted  int    `json:"deleted"`
	Status   string `json:"status,omitempty"`
	Verified bool   `json:"verified"`
}

// effectCommandCompleted is the reported effect of a completed write command.
const effectCommandCompleted = "command_completed"

// probeFailure marks a completed probe that reported a failed result. It is a
// finished observation, not an unknown outcome.
type probeFailure struct{ details *ProbeDetails }

func (e *probeFailure) Error() string { return "the API probe reported a failure" }

// NewSuccess builds a successful envelope.
func NewSuccess(operation, contextName string, data any) *Result {
	return &Result{
		SchemaVersion: SchemaVersion,
		Operation:     operation,
		Context:       contextName,
		Success:       true,
		Data:          data,
	}
}

// NewFailure builds a failed envelope.
func NewFailure(operation, contextName, code, message, suggestion string) *Result {
	return &Result{
		SchemaVersion: SchemaVersion,
		Operation:     operation,
		Context:       contextName,
		Success:       false,
		Error: &Error{
			Code:       code,
			Message:    message,
			Suggestion: suggestion,
		},
	}
}

// callToolResult renders the envelope for the MCP transport. The structured
// object and its serialized fallback always agree, and IsError always matches
// the envelope's success flag.
func (r *Result) callToolResult() (*mcp.CallToolResult, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	result := mcp.NewToolResultStructured(r, string(payload))
	result.IsError = !r.Success
	return result, nil
}

// ---------------------------------------------------------------------------
// Projected result data
//
// Every type below is an explicit allow-list. Decoding into these types drops
// unknown fields by construction, and the projections omit credential values,
// literal header values, sensitive schema defaults or examples, and raw URLs.
// ---------------------------------------------------------------------------

// ContextItem is one projected saved context. Connection and authentication
// settings are never included.
type ContextItem struct {
	Name               string `json:"name"`
	SelectedForSession bool   `json:"selectedForSession"`
}

// ContextListData is the projected context inventory.
type ContextListData struct {
	Items []ContextItem `json:"items"`
}

// APIItem is one projected registered API.
type APIItem struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name"`
	Method           string `json:"method,omitempty"`
	Module           string `json:"module,omitempty"`
	Status           string `json:"status,omitempty"`
	Description      string `json:"description,omitempty"`
	EndpointLabel    string `json:"endpointLabel,omitempty"`
	AuthType         string `json:"authType,omitempty"`
	AuthHeader       string `json:"authHeader,omitempty"`
	CredentialRef    string `json:"credentialRef,omitempty"`
	CredentialSource string `json:"credentialSource,omitempty"`
}

// APIListData is the projected API inventory.
type APIListData struct {
	Items []APIItem `json:"items"`
}

// SchemaProperty is one projected input-schema declaration. Defaults and
// examples are omitted because they may carry credential material, while the
// declaration itself is preserved so a property named password or token stays
// visible.
type SchemaProperty struct {
	Type        string   `json:"type,omitempty"`
	ItemsType   string   `json:"itemsType,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

// ReferenceItem is a projected logical resource reference.
type ReferenceItem struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// ResourceItem is one projected tool, plugin, or binding. It is an inspection
// view: credential values, literal header values, raw URLs, and arbitrary
// configuration values are omitted, so it is not round-trippable.
type ResourceItem struct {
	APIVersion       string                    `json:"apiVersion,omitempty"`
	Kind             string                    `json:"kind,omitempty"`
	Name             string                    `json:"name"`
	Version          string                    `json:"version,omitempty"`
	Module           string                    `json:"module,omitempty"`
	Description      string                    `json:"description,omitempty"`
	WhenToUse        []string                  `json:"whenToUse,omitempty"`
	Status           string                    `json:"status,omitempty"`
	Active           *bool                     `json:"active,omitempty"`
	Serving          *bool                     `json:"serving,omitempty"`
	Type             string                    `json:"type,omitempty"`
	Method           string                    `json:"method,omitempty"`
	EndpointLabel    string                    `json:"endpointLabel,omitempty"`
	AuthType         string                    `json:"authType,omitempty"`
	AuthHeader       string                    `json:"authHeader,omitempty"`
	CredentialRef    string                    `json:"credentialRef,omitempty"`
	CredentialSource string                    `json:"credentialSource,omitempty"`
	DataClass        string                    `json:"dataClass,omitempty"`
	AllowedRoles     []string                  `json:"allowedRoles,omitempty"`
	InputProperties  map[string]SchemaProperty `json:"inputProperties,omitempty"`
	RequiredInputs   []string                  `json:"requiredInputs,omitempty"`
	Phase            string                    `json:"phase,omitempty"`
	Priority         *int                      `json:"priority,omitempty"`
	FailurePolicy    string                    `json:"failurePolicy,omitempty"`
	PluginRef        *ReferenceItem            `json:"pluginRef,omitempty"`
	ToolRef          *ReferenceItem            `json:"toolRef,omitempty"`
}

// ResourceData is a named resource or a resource list. Inspection results are
// explicitly marked non-round-trippable.
type ResourceData struct {
	Item     *ResourceItem  `json:"item,omitempty"`
	Items    []ResourceItem `json:"items,omitempty"`
	Redacted bool           `json:"redacted"`
}

// TokenItem is one projected API token. Only existing metadata is included; any
// token, secret, or hash field is dropped by the decoder.
type TokenItem struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes,omitempty"`
	Roles     []string   `json:"roles,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// TokenListData is the projected token inventory.
type TokenListData struct {
	Items []TokenItem `json:"items"`
}

// CacheStatsData is the projected cache summary.
type CacheStatsData struct {
	ExactKeys   int64  `json:"exactKeys"`
	RedisMemory string `json:"redisMemory,omitempty"`
}

// LogStatsData is the projected log summary. It carries counts only, never log
// messages or raw event fields.
type LogStatsData struct {
	LevelCounts map[string]int `json:"levelCounts"`
	ToolCounts  map[string]int `json:"toolCounts"`
}

// ValidationData is the summary of one locally validated resource.
type ValidationData struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Valid   bool   `json:"valid"`
}

// GenerateData carries generated manifests. Generation returns the executable
// document after typed credential and URL checks, because applying it is the
// purpose of the operation.
type GenerateData struct {
	Manifests []json.RawMessage `json:"manifests"`
}

// ---------------------------------------------------------------------------
// Wire decoders
// ---------------------------------------------------------------------------

type wireContextList struct {
	Items []struct {
		Name    string `json:"name"`
		Current bool   `json:"current"`
	} `json:"items"`
}

type wireAPIList struct {
	Items []struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		URL              string `json:"url"`
		Method           string `json:"method"`
		Module           string `json:"module"`
		Description      string `json:"description"`
		Status           string `json:"status"`
		AuthType         string `json:"authType"`
		AuthHeader       string `json:"authHeader"`
		CredentialRef    string `json:"credentialRef"`
		CredentialSource string `json:"credentialSource"`
	} `json:"items"`
}

type wireMetadata struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Module    string `json:"module"`
	Status    string `json:"status"`
	IsActive  bool   `json:"isActive"`
	IsServing bool   `json:"isServing"`
}

type wireProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Enum        []any  `json:"enum"`
	Items       *struct {
		Type string `json:"type"`
	} `json:"items"`
}

type wireTool struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   wireMetadata `json:"metadata"`
	Spec       struct {
		Description struct {
			Short     string   `json:"short"`
			WhenToUse []string `json:"whenToUse"`
		} `json:"description"`
		InputSchema struct {
			Type       string                  `json:"type"`
			Required   []string                `json:"required"`
			Properties map[string]wireProperty `json:"properties"`
		} `json:"inputSchema"`
		Execution struct {
			Method   string `json:"method"`
			Endpoint string `json:"endpoint"`
		} `json:"execution"`
		Security struct {
			AuthType         string   `json:"authType"`
			AuthHeader       string   `json:"authHeader"`
			CredentialRef    string   `json:"credentialRef"`
			CredentialSource string   `json:"credentialSource"`
			DataClass        string   `json:"dataClass"`
			AllowedRoles     []string `json:"allowedRoles"`
		} `json:"security"`
	} `json:"spec"`
}

type wireToolList struct {
	Tools []wireTool `json:"tools"`
}

type wirePlugin struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   wireMetadata `json:"metadata"`
	Spec       struct {
		Type     string `json:"type"`
		Endpoint string `json:"endpoint"`
	} `json:"spec"`
}

type wirePluginList struct {
	Plugins []wirePlugin `json:"plugins"`
}

type wireBinding struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name     string `json:"name"`
		IsActive bool   `json:"isActive"`
	} `json:"metadata"`
	Spec struct {
		PluginRef     ReferenceItem `json:"pluginRef"`
		ToolRef       ReferenceItem `json:"toolRef"`
		Phase         string        `json:"phase"`
		Priority      int           `json:"priority"`
		FailurePolicy string        `json:"failurePolicy"`
	} `json:"spec"`
}

type wireBindingList struct {
	Bindings []wireBinding `json:"bindings"`
}

type wireTokenList struct {
	Items []TokenItem `json:"items"`
}

// wireAPITest is the CLI's server-side probe result. Latency is a Go duration,
// so its unit is converted explicitly.
type wireAPITest struct {
	APIName     string `json:"api"`
	Status      string `json:"status"`
	Code        int    `json:"code"`
	Latency     int64  `json:"latency"`
	ContentType string `json:"contentType"`
	IsSuccess   bool   `json:"isSuccess"`
}

// wireFlush is the cache-flush result.
type wireFlush struct {
	Deleted int    `json:"deleted"`
	Status  string `json:"status"`
}

// wireRegistration carries only the replacement flag from a registration
// result; the registration body itself is never returned.
type wireRegistration struct {
	Replaced bool `json:"replaced"`
}

type wireCacheStats struct {
	Stats struct {
		ExactKeys   int64  `json:"exactKeys"`
		RedisMemory string `json:"redisMemory"`
	} `json:"stats"`
}

// wireChildError is the child's actionable error envelope. Only the code field
// is read: messages and suggestions stay adapter-owned.
type wireChildError struct {
	ErrorCode string `json:"error"`
}

// ---------------------------------------------------------------------------
// Projection helpers
// ---------------------------------------------------------------------------

// endpointLabel returns a neutral host label for an endpoint, or an empty
// string when the value is unusable. It never returns credentials, paths, or
// query values.
func endpointLabel(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Host
}

// normalizeEnum renders enum declarations as bounded strings.
func normalizeEnum(values []any) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if typed == "" || len(typed) > 128 {
				continue
			}
			result = append(result, typed)
		case nil:
			continue
		default:
			rendered := fmt.Sprint(typed)
			if len(rendered) > 128 {
				continue
			}
			result = append(result, rendered)
		}
	}
	return result
}

// boolPointer returns a pointer so an omitted field stays distinguishable from
// an explicit false.
func boolPointer(value bool) *bool { return &value }

func projectContextList(stdout []byte) (any, error) {
	var wire wireContextList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := ContextListData{Items: make([]ContextItem, 0, len(wire.Items))}
	for _, item := range wire.Items {
		if item.Name == "" {
			return nil, errors.New("context entry has no name")
		}
		data.Items = append(data.Items, ContextItem{Name: item.Name, SelectedForSession: item.Current})
	}
	return data, nil
}

func projectAPIList(stdout []byte) (any, error) {
	var wire wireAPIList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := APIListData{Items: make([]APIItem, 0, len(wire.Items))}
	for _, item := range wire.Items {
		if item.Name == "" {
			return nil, errors.New("API entry has no name")
		}
		data.Items = append(data.Items, APIItem{
			ID:               item.ID,
			Name:             item.Name,
			Method:           item.Method,
			Module:           item.Module,
			Status:           item.Status,
			Description:      item.Description,
			EndpointLabel:    endpointLabel(item.URL),
			AuthType:         item.AuthType,
			AuthHeader:       item.AuthHeader,
			CredentialRef:    item.CredentialRef,
			CredentialSource: item.CredentialSource,
		})
	}
	return data, nil
}

func projectToolResource(wire wireTool) (ResourceItem, error) {
	if wire.Metadata.Name == "" {
		return ResourceItem{}, errors.New("tool has no metadata.name")
	}
	item := ResourceItem{
		APIVersion:       wire.APIVersion,
		Kind:             wire.Kind,
		Name:             wire.Metadata.Name,
		Version:          wire.Metadata.Version,
		Module:           wire.Metadata.Module,
		Description:      wire.Spec.Description.Short,
		WhenToUse:        wire.Spec.Description.WhenToUse,
		Status:           wire.Metadata.Status,
		Active:           boolPointer(wire.Metadata.IsActive),
		Serving:          boolPointer(wire.Metadata.IsServing),
		Method:           strings.ToUpper(wire.Spec.Execution.Method),
		EndpointLabel:    endpointLabel(wire.Spec.Execution.Endpoint),
		AuthType:         wire.Spec.Security.AuthType,
		AuthHeader:       wire.Spec.Security.AuthHeader,
		CredentialRef:    wire.Spec.Security.CredentialRef,
		CredentialSource: wire.Spec.Security.CredentialSource,
		DataClass:        wire.Spec.Security.DataClass,
		AllowedRoles:     wire.Spec.Security.AllowedRoles,
		RequiredInputs:   wire.Spec.InputSchema.Required,
	}
	if properties := wire.Spec.InputSchema.Properties; len(properties) > 0 {
		required := make(map[string]bool, len(wire.Spec.InputSchema.Required))
		for _, name := range wire.Spec.InputSchema.Required {
			required[name] = true
		}
		item.InputProperties = make(map[string]SchemaProperty, len(properties))
		for name, property := range properties {
			projected := SchemaProperty{
				Type:        property.Type,
				Description: property.Description,
				Enum:        normalizeEnum(property.Enum),
				Required:    required[name],
			}
			if property.Items != nil {
				projected.ItemsType = property.Items.Type
			}
			item.InputProperties[name] = projected
		}
	}
	return item, nil
}

func projectToolList(stdout []byte) (any, error) {
	var wire wireToolList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := ResourceData{Items: make([]ResourceItem, 0, len(wire.Tools)), Redacted: true}
	for _, entry := range wire.Tools {
		item, err := projectToolResource(entry)
		if err != nil {
			return nil, err
		}
		data.Items = append(data.Items, item)
	}
	return data, nil
}

func projectToolItem(stdout []byte) (any, error) {
	var wire wireTool
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	item, err := projectToolResource(wire)
	if err != nil {
		return nil, err
	}
	return ResourceData{Item: &item, Redacted: true}, nil
}

func projectPluginResource(wire wirePlugin) (ResourceItem, error) {
	if wire.Metadata.Name == "" {
		return ResourceItem{}, errors.New("plugin has no metadata.name")
	}
	return ResourceItem{
		APIVersion:    wire.APIVersion,
		Kind:          wire.Kind,
		Name:          wire.Metadata.Name,
		Version:       wire.Metadata.Version,
		Type:          wire.Spec.Type,
		Active:        boolPointer(wire.Metadata.IsActive),
		EndpointLabel: endpointLabel(wire.Spec.Endpoint),
	}, nil
}

func projectPluginList(stdout []byte) (any, error) {
	var wire wirePluginList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := ResourceData{Items: make([]ResourceItem, 0, len(wire.Plugins)), Redacted: true}
	for _, entry := range wire.Plugins {
		item, err := projectPluginResource(entry)
		if err != nil {
			return nil, err
		}
		data.Items = append(data.Items, item)
	}
	return data, nil
}

func projectPluginItem(stdout []byte) (any, error) {
	var wire wirePlugin
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	item, err := projectPluginResource(wire)
	if err != nil {
		return nil, err
	}
	return ResourceData{Item: &item, Redacted: true}, nil
}

func projectBindingResource(wire wireBinding) (ResourceItem, error) {
	if wire.Metadata.Name == "" {
		return ResourceItem{}, errors.New("binding has no metadata.name")
	}
	priority := wire.Spec.Priority
	return ResourceItem{
		APIVersion:    wire.APIVersion,
		Kind:          wire.Kind,
		Name:          wire.Metadata.Name,
		Active:        boolPointer(wire.Metadata.IsActive),
		Phase:         wire.Spec.Phase,
		Priority:      &priority,
		FailurePolicy: wire.Spec.FailurePolicy,
		PluginRef:     &wire.Spec.PluginRef,
		ToolRef:       &wire.Spec.ToolRef,
	}, nil
}

func projectBindingList(stdout []byte) (any, error) {
	var wire wireBindingList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := ResourceData{Items: make([]ResourceItem, 0, len(wire.Bindings)), Redacted: true}
	for _, entry := range wire.Bindings {
		item, err := projectBindingResource(entry)
		if err != nil {
			return nil, err
		}
		data.Items = append(data.Items, item)
	}
	return data, nil
}

func projectBindingItem(stdout []byte) (any, error) {
	var wire wireBinding
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	item, err := projectBindingResource(wire)
	if err != nil {
		return nil, err
	}
	return ResourceData{Item: &item, Redacted: true}, nil
}

func projectTokenList(stdout []byte) (any, error) {
	var wire wireTokenList
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := TokenListData{Items: make([]TokenItem, 0, len(wire.Items))}
	for _, item := range wire.Items {
		if item.ID == "" {
			return nil, errors.New("token entry has no id")
		}
		data.Items = append(data.Items, item)
	}
	return data, nil
}

func projectCacheStats(stdout []byte) (any, error) {
	var wire wireCacheStats
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	return CacheStatsData{ExactKeys: wire.Stats.ExactKeys, RedisMemory: wire.Stats.RedisMemory}, nil
}

// projectAPITest converts a completed probe into typed data. A probe that
// reported a failed result is a completed observation, not an unknown outcome.
func projectAPITest(stdout []byte) (any, error) {
	var wire wireAPITest
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	details := &ProbeDetails{
		APIName:        wire.APIName,
		HTTPStatus:     wire.Code,
		ContentType:    wire.ContentType,
		LatencyMs:      durationToMilliseconds(wire.Latency),
		ProbeSucceeded: wire.IsSuccess,
	}
	if !wire.IsSuccess {
		return nil, &probeFailure{details: details}
	}
	return ProbeData{
		APIName:        details.APIName,
		HTTPStatus:     details.HTTPStatus,
		ContentType:    details.ContentType,
		LatencyMs:      details.LatencyMs,
		ProbeSucceeded: true,
	}, nil
}

// durationToMilliseconds converts a Go duration in nanoseconds.
func durationToMilliseconds(nanoseconds int64) int64 {
	if nanoseconds <= 0 {
		return 0
	}
	return nanoseconds / int64(time.Millisecond)
}

// knownLogLevels bounds the accepted level labels. An unexpected label is
// grouped rather than forwarded.
var knownLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "warning": true,
	"error": true, "fatal": true, "panic": true,
}

// unrecognizedLabel groups counts for values that are not bounded, known
// labels.
const unrecognizedLabel = "unrecognized"

func projectLogStats(stdout []byte) (any, error) {
	var wire struct {
		LevelCounts map[string]int `json:"levelCounts"`
		ToolCounts  map[string]int `json:"toolCounts"`
	}
	if err := decodeChildJSON(stdout, &wire); err != nil {
		return nil, err
	}
	data := LogStatsData{LevelCounts: map[string]int{}, ToolCounts: map[string]int{}}
	for label, count := range wire.LevelCounts {
		if knownLogLevels[strings.ToLower(label)] {
			data.LevelCounts[strings.ToLower(label)] += count
			continue
		}
		data.LevelCounts[unrecognizedLabel] += count
	}
	for label, count := range wire.ToolCounts {
		if isSafeLabel(label) {
			data.ToolCounts[label] += count
			continue
		}
		data.ToolCounts[unrecognizedLabel] += count
	}
	return data, nil
}

// isSafeLabel reports whether a count label is bounded and printable, so an
// arbitrary log value cannot travel through the summary.
func isSafeLabel(value string) bool {
	if value == "" || len(value) > maxIdentifierBytes {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// decodeChildJSON decodes one bounded, single-document child payload.
func decodeChildJSON(stdout []byte, target any) error {
	if len(stdout) == 0 {
		return errors.New("child produced no output")
	}
	decoder := json.NewDecoder(strings.NewReader(string(stdout)))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode child output: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("child output contained more than one document")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Failure classification
// ---------------------------------------------------------------------------

// childFailureCodes maps a recognized CLI error code to adapter-owned text.
// Only codes listed here cross the boundary; unknown codes become a generic
// failure so no child text is forwarded.
var childFailureCodes = map[string]struct {
	code       string
	message    string
	suggestion string
}{
	"CONTEXT_NOT_FOUND": {CodeNotFound,
		"the selected context is not configured",
		"Run bridgectl context list to see the configured contexts."},
	"RESOURCE_NOT_FOUND": {CodeNotFound,
		"the requested resource was not found",
		"Inspect the resource identity and retry."},
	"API_NOT_FOUND": {CodeNotFound,
		"the requested API is not registered in this context",
		"List the registered APIs before requesting this one."},
	"AUTHENTICATION_FAILED": {CodeAuthentication,
		"the ERPBridge deployment rejected the session credential",
		"Check the credential configured for the selected context."},
	"AUTHORIZATION_DENIED": {CodeAuthorizationDenied,
		"the ERPBridge deployment denied this operation",
		"Use a credential with the required scope or role."},
	"REGISTRY_CONFLICT": {CodeConflict,
		"a resource with that name already exists",
		"Choose another name, or replace it explicitly when that is intended."},
	"UPSTREAM_UNREACHABLE": {CodeUpstreamUnavailable,
		"the ERPBridge control plane could not be reached",
		"Check that ERPBridge is running and that the pinned endpoint is reachable."},
	"HEALTH_CHECK_FAILED": {CodeUpstreamUnavailable,
		"the ERPBridge control plane reported an unhealthy state",
		"Check the deployment health before retrying."},
	"VALIDATION_FAILED": {CodeInvalidArguments,
		"the submitted arguments or resource failed validation",
		"Correct the input and retry."},
	"INVALID_URL": {CodeInvalidArguments,
		"a provided URL was rejected",
		"Provide an absolute http or https URL without credentials in the userinfo."},
	"CONTROL_PLANE_URL_INVALID": {CodeInvalidArguments,
		"the configured control-plane URL is not usable",
		"Set the context endpoint to a host root such as https://host."},
	"INSECURE_TRANSPORT": {CodeInvalidArguments,
		"the operation was refused over an insecure transport",
		"Use HTTPS, or explicitly allow the local development host."},
	"LEGACY_REGISTRY": {CodeInvalidArguments,
		"the local registry requires a credential-safe migration",
		"Complete the registry migration before using this tool."},
	"MCP_CONTEXT_CHANGED": {CodeContextChanged,
		"the session selection no longer matches the saved configuration",
		"Restart the management MCP process to adopt the changed context, endpoint, or credential."},
	"NO_CONTEXT": {CodeContextChanged,
		"no context is available for this session",
		"Select a configured context and restart the management MCP process."},
	"MISCONFIGURED_CONTEXT": {CodeContextChanged,
		"the selected context is incomplete",
		"Provide the missing endpoint in the context configuration."},
}

// exitCodeFailures maps a recognized CLI exit code to adapter-owned text when
// the child produced no error envelope.
var exitCodeFailures = map[int]struct {
	code       string
	message    string
	suggestion string
}{
	2: {CodeInvalidArguments, "the child rejected its arguments", "Correct the input and retry."},
	3: {CodeNotFound, "the requested resource was not found", "Inspect the resource identity and retry."},
	4: {CodeAuthentication, "the ERPBridge deployment rejected the session credential", "Check the credential configured for the selected context."},
	5: {CodeConflict, "a resource with that name already exists", "Choose another name, or replace it explicitly when that is intended."},
	6: {CodeUpstreamUnavailable, "the ERPBridge control plane could not be reached", "Check that ERPBridge is running and that the pinned endpoint is reachable."},
	7: {CodeContextChanged, "the session precondition is no longer satisfied", "Restart the management MCP process to adopt the changed configuration."},
}

// childErrorCode extracts a recognized error code from bounded child output.
func childErrorCode(stdout, stderr []byte) (string, bool) {
	for _, stream := range [][]byte{stdout, stderr} {
		candidate := strings.TrimSpace(string(stream))
		if candidate == "" {
			continue
		}
		if index := strings.LastIndex(candidate, "{"); index >= 0 {
			candidate = candidate[index:]
		}
		var envelope wireChildError
		if err := json.Unmarshal([]byte(candidate), &envelope); err != nil {
			continue
		}
		if envelope.ErrorCode != "" {
			return envelope.ErrorCode, true
		}
	}
	return "", false
}
