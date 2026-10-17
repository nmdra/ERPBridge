package climcp

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// SchemaVersion is the version of the adapter result envelope.
const SchemaVersion = 1

// Stable adapter result codes. These describe check outcomes for the calling
// agent; they are not CLI exit codes or protocol error codes.
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
