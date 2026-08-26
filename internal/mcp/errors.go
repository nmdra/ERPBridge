package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
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
