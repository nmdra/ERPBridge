package cli

import (
	"fmt"
	"net/url"
	"strings"
)

// Exit codes for agent compatibility.
const (
	// CodeSuccess indicates the command completed successfully.
	CodeSuccess = 0
	// CodeGeneralErr indicates an unspecified error occurred.
	CodeGeneralErr = 1
	// CodeBadArgs indicates invalid arguments were provided.
	CodeBadArgs = 2
	// CodeNotFound indicates a requested resource was not found.
	CodeNotFound = 3
	// CodeAuthFail indicates an authentication failure.
	CodeAuthFail = 4
	// CodeConflict indicates a resource conflict.
	CodeConflict = 5
	// CodeTimeout indicates the operation timed out.
	CodeTimeout = 6
	// CodePrecondFail indicates a precondition for the command was not met.
	CodePrecondFail = 7
)

// AgentActionableError represents an error that can be programmatically
// handled by an AI agent or other automated system.
type AgentActionableError struct {
	// ErrorCode is a machine-readable string identifying the error type.
	ErrorCode string `json:"error"`
	// Message is a human-readable description of the error.
	Message string `json:"message"`
	// Suggestion provides a possible fix or next step.
	Suggestion string `json:"suggestion,omitempty"`
	// Code is the numeric exit code associated with this error.
	Code int `json:"code"`
}

// Error implements the error interface.
func (e *AgentActionableError) Error() string {
	return fmt.Sprintf("[%s] %s (Exit Code: %d)", e.ErrorCode, e.Message, e.Code)
}

// NewError creates a new AgentActionableError with the provided details.
func NewError(code int, errCode string, message string, suggestion string) *AgentActionableError {
	return &AgentActionableError{
		Code:       code,
		ErrorCode:  errCode,
		Message:    message,
		Suggestion: suggestion,
	}
}

// ValidateServerURL checks if the provided URL is non-empty and has a valid protocol.
func ValidateServerURL(url, serverType, contextName string) error {
	if url == "" {
		return NewError(CodePrecondFail, "MISCONFIGURED_CONTEXT",
			fmt.Sprintf("%s URL is not set for context %q", serverType, contextName),
			fmt.Sprintf("Update it using 'BRIDGE_%s_SERVER' environment variable or in ~/.bridgectl/config.yaml", serverType))
	}

	if !hasProtocol(url) {
		return NewError(CodeBadArgs, "INVALID_URL",
			fmt.Sprintf("Invalid %s URL %q: missing protocol (http:// or https://)", serverType, url),
			"Ensure the server URL starts with http:// or https://")
	}

	return nil
}

func hasProtocol(url string) bool {
	return (len(url) >= 7 && url[:7] == "http://") || (len(url) >= 8 && url[:8] == "https://")
}

// controlPlaneRoot returns the URL used for REST/control-plane requests. The
// configured MCP URL may point at the streamable transport for compatibility,
// but no other path is accepted because it is ambiguous which endpoint is
// intended.
func controlPlaneRoot(raw, contextName string) (string, error) {
	if err := ValidateServerURL(raw, "MCP", contextName); err != nil {
		return "", err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", NewError(CodeBadArgs, "CONTROL_PLANE_URL_INVALID",
			"the configured MCP URL is not a valid control-plane root",
			"Set mcp-server to an http(s) host root, or use only the exact /mcp or /mcp/ suffix for the MCP transport.")
	}

	switch parsed.Path {
	case "", "/", "/mcp", "/mcp/":
		parsed.Path = ""
		parsed.RawPath = ""
	default:
		return "", NewError(CodeBadArgs, "CONTROL_PLANE_URL_INVALID",
			"the configured MCP URL has a non-transport path and cannot be used for control-plane requests",
			"Set mcp-server to the host root, or use only the exact /mcp or /mcp/ suffix for the MCP transport.")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func unreachableControlPlaneError(_ error) error {
	return NewError(CodeTimeout, "UPSTREAM_UNREACHABLE",
		"the ERPBridge control plane could not be reached",
		"Check the selected context's control-plane root and confirm ERPBridge is running.")
}
