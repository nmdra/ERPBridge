package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/faults"
)

// Stable control-plane error identifiers. These values are part of the
// machine-readable HTTP contract and must not contain implementation details.
const (
	ErrorContextNotFound      = "CONTEXT_NOT_FOUND"
	ErrorLegacyRegistry       = "LEGACY_REGISTRY"
	ErrorRegistryConflict     = "REGISTRY_CONFLICT"
	ErrorControlPlaneURL      = "CONTROL_PLANE_URL_INVALID"
	ErrorValidationFailed     = "VALIDATION_FAILED"
	ErrorAuthenticationFailed = "AUTHENTICATION_FAILED"
	ErrorAuthorizationDenied  = "AUTHORIZATION_DENIED"
	ErrorUpstreamUnreachable  = "UPSTREAM_UNREACHABLE"
	ErrorInsecureTransport    = "INSECURE_TRANSPORT"
	ErrorHealthCheckFailed    = "HEALTH_CHECK_FAILED"
	ErrorReconciliationFailed = "RECONCILIATION_FAILED"
	ErrorResourceNotFound     = "RESOURCE_NOT_FOUND"
	ErrorMethodNotAllowed     = "METHOD_NOT_ALLOWED"
	ErrorAPIProbeFailed       = "API_PROBE_FAILED"
	ErrorRateLimited          = "RATE_LIMITED"
)

type controlPlaneErrorEnvelope struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	Code       int    `json:"code"`
}

// writeControlPlaneError is the sole writer for control-plane failures. Keep
// its inputs static and safe: callers must not pass upstream bodies, secrets,
// URLs, or internal error strings.
func writeControlPlaneError(w http.ResponseWriter, status int, code, message, suggestion string) {
	if status < http.StatusBadRequest {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(controlPlaneErrorEnvelope{
		Error: code, Message: message, Suggestion: suggestion, Code: status,
	})
}

func writeControlPlaneInternalError(w http.ResponseWriter, status int, code, suggestion string) {
	writeControlPlaneError(w, status, code, "the control-plane operation could not be completed", suggestion)
}

func safeJSONValidationMessage(kind string, err error) string {
	message := "the " + kind + " resource is not valid JSON"
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unknown field") {
		return message + ": unknown field"
	}
	return message
}

const defaultToolExecutionErrorMessage = "the tool could not complete; check server logs before retrying"

func newToolExecutionResult(err error) *mcp.CallToolResult {
	fault, ok := faults.As(err)
	if !ok {
		message := defaultToolExecutionErrorMessage
		if errors.Is(err, ErrPluginProcessingFailed) {
			message = pluginProcessingFailureMessage
		}
		fault = faults.New(faults.KindInternal, message, false, 0, err)
	}

	result := mcp.NewToolResultError(fault.Error())
	metadata := map[string]any{
		"type":      faultTypeName(fault.Kind),
		"retryable": fault.Retryable,
	}
	if fault.RetryAfter > 0 {
		metadata["retryAfterMs"] = fault.RetryAfter.Milliseconds()
	}
	result.Meta = &mcp.Meta{AdditionalFields: map[string]any{
		"com.erpbridge/error": metadata,
	}}
	return result
}

func toolResultError(value any) error {
	if err, ok := value.(error); ok {
		return err
	}
	if message, ok := value.(string); ok && message == pluginProcessingFailureMessage {
		return faults.New(faults.KindInternal, pluginProcessingFailureMessage, false, 0, ErrPluginProcessingFailed)
	}
	return faults.New(faults.KindInternal, defaultToolExecutionErrorMessage, false, 0, errors.New("unclassified tool result failure"))
}

func faultTypeName(kind faults.Kind) string {
	switch kind {
	case faults.KindInvalidInput:
		return "invalid_input"
	case faults.KindNotFound:
		return "not_found"
	case faults.KindPermissionDenied:
		return "permission_denied"
	case faults.KindRateLimited:
		return "rate_limit"
	case faults.KindConcurrencyLimited:
		return "concurrency_limit"
	case faults.KindDependencyTimeout:
		return "dependency_timeout"
	case faults.KindDependencyUnavailable:
		return "dependency_unavailable"
	case faults.KindConflict:
		return "conflict"
	default:
		return dataClassInternal
	}
}

func retryAfterSeconds(delay time.Duration) int64 {
	if delay <= 0 {
		return 1
	}
	seconds := int64(delay / time.Second)
	if delay%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func retryAfterText(delay time.Duration) string {
	seconds := retryAfterSeconds(delay)
	unit := "seconds"
	if seconds == 1 {
		unit = "second"
	}
	return fmt.Sprintf("%d %s", seconds, unit)
}

func toolResultErrorType(result *mcp.CallToolResult) string {
	if result != nil && result.Meta != nil {
		if metadata, ok := result.Meta.AdditionalFields["com.erpbridge/error"].(map[string]any); ok {
			if typeName, ok := metadata["type"].(string); ok && typeName != "" {
				return typeName
			}
		}
	}
	return dataClassInternal
}

func writeToolExecutionHTTPError(w http.ResponseWriter, result *mcp.CallToolResult) {
	status := http.StatusBadGateway
	code := ErrorUpstreamUnreachable
	suggestion := "check the upstream ERP service and tool configuration"
	typeName := dataClassInternal
	if result != nil && result.Meta != nil {
		if metadata, ok := result.Meta.AdditionalFields["com.erpbridge/error"].(map[string]any); ok {
			typeName, _ = metadata["type"].(string)
			if retryAfter, ok := metadata["retryAfterMs"].(int64); ok && retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(time.Duration(retryAfter)*time.Millisecond), 10))
			}
		}
	}
	switch typeName {
	case "invalid_input":
		status, code, suggestion = http.StatusBadRequest, ErrorValidationFailed, "review the tool arguments"
	case "not_found":
		status, code, suggestion = http.StatusNotFound, ErrorResourceNotFound, "check the requested ERP resource"
	case "permission_denied":
		status, code, suggestion = http.StatusForbidden, ErrorAuthorizationDenied, "verify access to the ERP operation"
	case "rate_limit", "concurrency_limit":
		status, code, suggestion = http.StatusTooManyRequests, ErrorRateLimited, "retry after the indicated delay"
	case "dependency_timeout":
		status, code, suggestion = http.StatusGatewayTimeout, ErrorUpstreamUnreachable, "retry after the dependency is available"
	case "dependency_unavailable":
		status, code, suggestion = http.StatusServiceUnavailable, ErrorUpstreamUnreachable, "retry after the dependency is available"
	case "conflict":
		status, code, suggestion = http.StatusConflict, ErrorValidationFailed, "review the current resource state before retrying"
	case dataClassInternal:
		status, code, suggestion = http.StatusInternalServerError, ErrorHealthCheckFailed, "check server logs before retrying"
	}
	writeControlPlaneError(w, status, code, "the tool execution failed", suggestion)
}
