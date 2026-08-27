package mcp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resourceEndpoint = "/inventory/stock"

func TestResource_Execute(t *testing.T) {
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, ep connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			assert.Equal(t, http.MethodGet, ep.Method)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"stock": 100}`)),
			}, nil
		},
	}

	r := &Resource{
		Name:        "inventory-stock",
		Description: "Stock levels",
		URITemplate: "erp://inventory/stock",
		MimeType:    "application/json",
		Execution: Execution{
			Method:   http.MethodGet,
			Endpoint: resourceEndpoint,
		},
	}

	res, err := r.Execute(context.Background(), "erp://inventory/stock", mockConn)
	assert.NoError(t, err)
	assert.Equal(t, `{"stock": 100}`, res)
}

func TestResource_ExecuteUsesFileCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ERPBRIDGE_CREDENTIALS_DIR", dir)
	credentialPath := filepath.Join(dir, "ERP_RESOURCE_FILE_KEY")
	require.NoError(t, os.WriteFile(credentialPath, []byte("resource-file-value-a"), 0600))
	var received []string
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, ep connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			received = append(received, ep.Auth.Key)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"stock": 100}`))}, nil
		},
	}
	r := &Resource{
		Name:        "inventory-file-stock",
		URITemplate: "erp://inventory/file-stock",
		Execution:   Execution{Method: http.MethodGet, Endpoint: resourceEndpoint},
		Security:    Security{AuthType: "bearer", CredentialRef: "ERP_RESOURCE_FILE_KEY", CredentialSource: "file"}, // #nosec G101 -- logical credential reference, not a secret.
	}
	_, err := r.Execute(context.Background(), r.URITemplate, mockConn)
	require.NoError(t, err)
	temporary := filepath.Join(dir, ".ERP_RESOURCE_FILE_KEY.next")
	require.NoError(t, os.WriteFile(temporary, []byte("resource-file-value-b"), 0600))
	require.NoError(t, os.Rename(temporary, credentialPath))
	_, err = r.Execute(context.Background(), r.URITemplate, mockConn)
	require.NoError(t, err)
	assert.Equal(t, []string{"resource-file-value-a", "resource-file-value-b"}, received)
}

func TestResource_Execute_NoEndpoint(t *testing.T) {
	r := &Resource{
		Name: "invalid-resource",
	}
	_, err := r.Execute(context.Background(), "erp://invalid", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoint configuration")
}

func TestResource_Execute_MissingCredentialRefFailsClosed(t *testing.T) {
	t.Setenv("ERPBRIDGE_RESOURCE_SECRET", "")
	called := false
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			called = true
			return nil, nil
		},
	}

	r := &Resource{
		Name:      "secret-resource",
		Execution: Execution{Endpoint: resourceEndpoint},
		// #nosec G101 -- this is an environment-variable reference used by the test.
		Security: Security{CredentialRef: "ERPBRIDGE_RESOURCE_SECRET"},
	}

	_, err := r.Execute(context.Background(), "erp://inventory/stock", mockConn)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "credential reference")
	assert.False(t, called)
}

func TestResource_Execute_UsesERPBaseURLForRelativeEndpoint(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "https://erp.example.test/api")
	var gotPath string
	mockConn := &MockConnector{
		CallFunc: func(_ context.Context, ep connector.EndpointConfig, _ url.Values, _ io.Reader) (*http.Response, error) {
			gotPath = ep.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
			}, nil
		},
	}

	r := &Resource{
		Name:      "base-url-resource",
		Execution: Execution{Endpoint: resourceEndpoint},
	}

	_, err := r.Execute(context.Background(), "erp://inventory/stock", mockConn)

	assert.NoError(t, err)
	assert.Equal(t, "https://erp.example.test/api/inventory/stock", gotPath)
}
