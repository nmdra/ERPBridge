package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nmdra/ERPBridge/internal/connector"
)

// Resource represents a read-only data source that can be accessed by an AI agent.
type Resource struct {
	// Name is the unique identifier for the resource.
	Name string `json:"name"`
	// Description provides a human-readable explanation of the resource content.
	Description string `json:"description"`
	// URITemplate defines the pattern used to access this resource.
	URITemplate string `json:"uriTemplate"`
	// MimeType specifies the format of the resource content (e.g., "text/markdown").
	MimeType string `json:"mimeType,omitempty"`
	// Execution defines how to fetch the resource.
	Execution Execution `json:"execution"`
	// Security defines the auth for the resource.
	Security Security `json:"security"`
}

// Execute fetches the resource content from the underlying ERP system.
func (r *Resource) Execute(ctx context.Context, _ string, conn ERPConnector) (string, error) {
	if r.Execution.Endpoint == "" {
		return "", fmt.Errorf("resource %s has no endpoint configuration", r.Name)
	}

	cred, err := resolveCredential(r.Security.CredentialRef, r.Security.CredentialSource)
	if err != nil {
		return "", err
	}

	ep := connector.EndpointConfig{
		Method:  http.MethodGet,
		Path:    r.Execution.Endpoint,
		BaseURL: "",
		Auth: connector.AuthConfig{
			Type: r.Security.AuthType,
			Key:  cred,
		},
	}

	// Handle relative paths
	if !strings.HasPrefix(ep.Path, "http") {
		baseURL := strings.TrimSuffix(os.Getenv("ERP_BASE_URL"), "/")
		if baseURL == "" {
			baseURL = "http://localhost:8081"
		}
		ep.Path = baseURL + "/" + strings.TrimPrefix(ep.Path, "/")
	}

	resp, err := conn.Call(ctx, ep, nil, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
