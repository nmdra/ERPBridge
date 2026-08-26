package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"

	"github.com/stretchr/testify/require"
)

func TestWriteControlPlaneErrorUsesStableSafeEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeControlPlaneError(recorder, http.StatusUnprocessableEntity, ErrorValidationFailed, "request is invalid", "review the resource schema")

	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, ErrorValidationFailed, envelope["error"])
	require.Equal(t, "request is invalid", envelope["message"])
	require.Equal(t, "review the resource schema", envelope["suggestion"])
	require.Equal(t, float64(http.StatusUnprocessableEntity), envelope["code"])
}

func TestWriteControlPlaneErrorNeverIncludesInternalDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeControlPlaneError(recorder, http.StatusBadGateway, ErrorUpstreamUnreachable, "ERP endpoint could not be reached", "check the ERP endpoint and network")

	require.NotContains(t, recorder.Body.String(), "Authorization")
	require.NotContains(t, recorder.Body.String(), "stack")
	require.NotContains(t, recorder.Body.String(), "https://secret.example")
}

func TestAPIProbeErrorsUseSafeEnvelope(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, apiProbePath, bytes.NewBufferString(`{"url":"","method":"GET"}`))
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	var envelope controlPlaneErrorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, ErrorValidationFailed, envelope.Error)
	require.NotEmpty(t, envelope.Suggestion)
	require.Equal(t, http.StatusUnprocessableEntity, envelope.Code)
}

func TestAuthErrorsUseSafeEnvelope(t *testing.T) {
	t.Setenv(authTokenEnv, "admin-secret")
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var envelope controlPlaneErrorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, ErrorAuthenticationFailed, envelope.Error)
	require.NotContains(t, recorder.Body.String(), "admin-secret")
}

func TestAPIProbeRejectsInsecureCredentialTransportSafely(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	t.Setenv("PROBE_SECRET", "secret-value")
	t.Setenv("INSECURE_AUTH_ALLOWED_HOSTS", "")
	s := NewServer(connector.NewClient(logger.Init()), nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	mux := http.NewServeMux()
	s.ServeHTTP(mux, "")

	payload := []byte(`{"url":"http://example.invalid/items","method":"GET","authType":"bearer","credentialRef":"PROBE_SECRET"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, apiProbePath, bytes.NewReader(payload))
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	var envelope controlPlaneErrorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, ErrorInsecureTransport, envelope.Error)
	require.NotContains(t, recorder.Body.String(), "secret-value")
}
