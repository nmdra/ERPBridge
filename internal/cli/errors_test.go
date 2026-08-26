package cli

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"

	"github.com/stretchr/testify/require"
)

func TestAgentActionableError_Error(t *testing.T) {
	err := &AgentActionableError{
		ErrorCode: "TEST_ERR",
		Message:   "Something went wrong",
		Code:      CodeBadArgs,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "TEST_ERR") {
		t.Errorf("Error message should contain ErrorCode, got %v", errMsg)
	}
	if !strings.Contains(errMsg, "Something went wrong") {
		t.Errorf("Error message should contain Message, got %v", errMsg)
	}
}

func TestControlPlaneRootNormalizesMCPTransportSuffix(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "http://localhost:8080", want: "http://localhost:8080"},
		{raw: "http://localhost:8080/", want: "http://localhost:8080"},
		{raw: "http://localhost:8080/mcp", want: "http://localhost:8080"},
		{raw: "http://localhost:8080/mcp/", want: "http://localhost:8080"},
	} {
		t.Run(test.raw, func(t *testing.T) {
			got, err := controlPlaneRoot(test.raw, "test")
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestControlPlaneRootRejectsNonMCPPath(t *testing.T) {
	for _, raw := range []string{"http://localhost:8080/api", "http://localhost:8080/mcp/tools", "http://localhost:8080/mcp//"} {
		_, err := controlPlaneRoot(raw, "test")
		require.Error(t, err)
		var actionable *AgentActionableError
		require.ErrorAs(t, err, &actionable)
		require.Equal(t, "CONTROL_PLANE_URL_INVALID", actionable.ErrorCode)
		require.Contains(t, actionable.Suggestion, "/mcp/")
	}
}

func TestNewError(t *testing.T) {
	err := NewError(CodeNotFound, "NOT_FOUND", "Item not found", "Try another ID")

	if err.Code != CodeNotFound {
		t.Errorf("Expected code %d, got %d", CodeNotFound, err.Code)
	}
	if err.ErrorCode != "NOT_FOUND" {
		t.Errorf("Expected ErrorCode NOT_FOUND, got %v", err.ErrorCode)
	}
	if err.Suggestion != "Try another ID" {
		t.Errorf("Expected Suggestion 'Try another ID', got %v", err.Suggestion)
	}
}

func TestMapRemoteErrorToAgentActionableError(t *testing.T) {
	remote := &bridgeclient.RemoteError{ErrorCode: "AUTHORIZATION_DENIED", Message: "access denied", Suggestion: "use an authorized token", Code: http.StatusForbidden, Status: http.StatusForbidden}
	err := mapRemoteError(remote)
	var actionable *AgentActionableError
	if !errors.As(err, &actionable) {
		t.Fatalf("error = %T, want AgentActionableError", err)
	}
	if actionable.ErrorCode != remote.ErrorCode || actionable.Code != CodeAuthFail {
		t.Fatalf("actionable error = %+v", actionable)
	}
}

func TestRenderActionableErrorHumanAndJSON(t *testing.T) {
	errorValue := NewError(CodeConflict, "REGISTRY_CONFLICT", "definition already exists", "use --force")
	var human bytes.Buffer
	if err := renderActionableError(&human, errorValue, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human.String(), "REGISTRY_CONFLICT") || !strings.Contains(human.String(), "Suggestion: use --force") {
		t.Fatalf("human error = %q", human.String())
	}
	var machine bytes.Buffer
	if err := renderActionableError(&machine, errorValue, "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(machine.String(), `"error": "REGISTRY_CONFLICT"`) || !strings.Contains(machine.String(), `"code": 5`) {
		t.Fatalf("JSON error = %q", machine.String())
	}
}
