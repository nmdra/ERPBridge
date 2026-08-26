package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/security"
)

const (
	apiProbePath         = "/api/apis/test"
	apiProbeResourcePath = "/apis/erpbridge.io/v1/apis/test"
	maxAPIProbeBody      = 8 << 10
	maxAPIProbeURL       = 2048
	maxAPIProbeMethod    = 16
	maxAPIProbeAuth      = 32
	maxAPIProbeHeader    = 128
	maxAPIProbeCredRef   = 128
)

// APIProbeRequest is the bounded, non-secret request accepted by the API
// probe. credentialRef is an environment-variable name, never a credential.
type APIProbeRequest struct {
	URL           string `json:"url"`
	Method        string `json:"method"`
	AuthType      string `json:"authType,omitempty"`
	AuthHeader    string `json:"authHeader,omitempty"`
	CredentialRef string `json:"credentialRef,omitempty"`
}

// APIProbeResponse is the complete probe result. It intentionally contains no
// upstream body or headers.
type APIProbeResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Latency     int64  `json:"latency"`
	Success     bool   `json:"success"`
}

func (s *Server) handleAPIProbe(w http.ResponseWriter, r *http.Request) {
	var request APIProbeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIProbeBody))
	if err := decoder.Decode(&request); err != nil {
		writeAPIProbeError(w, http.StatusBadRequest, "invalid API probe request")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeAPIProbeError(w, http.StatusBadRequest, "API probe request must contain one JSON object")
		return
	}
	if err := validateAPIProbeRequest(request); err != nil {
		writeAPIProbeError(w, http.StatusUnprocessableEntity, "invalid API probe request")
		return
	}
	if insecureAPIProbeRequest(request) {
		writeControlPlaneError(w, http.StatusUnprocessableEntity, ErrorInsecureTransport, "the API endpoint cannot receive credentials over this transport", "use HTTPS or explicitly allow the local development host")
		return
	}
	if s.connector == nil {
		writeAPIProbeError(w, http.StatusServiceUnavailable, "ERP connector is unavailable")
		return
	}

	tool := &Tool{Spec: ToolSpec{
		Execution: Execution{Method: request.Method, Endpoint: request.URL},
		Security:  Security{AuthType: request.AuthType, CredentialRef: request.CredentialRef},
	}}
	ep, queryParams, body, err := tool.prepareERPCall(nil)
	if err != nil {
		if insecureAPIProbeRequest(request) {
			writeControlPlaneError(w, http.StatusUnprocessableEntity, ErrorInsecureTransport, "the API endpoint cannot receive credentials over this transport", "use HTTPS or explicitly allow the local development host")
			return
		}
		writeAPIProbeError(w, http.StatusUnprocessableEntity, "API credential or endpoint could not be prepared")
		return
	}
	ep.Auth.Header = request.AuthHeader

	start := time.Now()
	response, err := probeERP(r, s.connector, ep, queryParams, body)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeAPIProbeError(w, http.StatusBadGateway, "ERP endpoint could not be reached")
		return
	}
	if response == nil {
		writeAPIProbeError(w, http.StatusBadGateway, "ERP endpoint returned no response")
		return
	}
	if response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}

	result := APIProbeResponse{
		Status:      response.StatusCode,
		ContentType: safeContentType(response.Header.Get("Content-Type")),
		Latency:     latency,
		Success:     response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func insecureAPIProbeRequest(request APIProbeRequest) bool {
	if request.CredentialRef == "" || strings.TrimSpace(os.Getenv(request.CredentialRef)) == "" {
		return false
	}
	parsed, err := url.Parse(request.URL)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	_, _, err = security.ValidateOutboundTransport(parsed, true)
	return err != nil
}

func validateAPIProbeRequest(request APIProbeRequest) error {
	if request.URL == "" || len(request.URL) > maxAPIProbeURL {
		return errors.New("url is required and bounded")
	}
	if request.Method == "" || len(request.Method) > maxAPIProbeMethod {
		return errors.New("method is required and bounded")
	}
	if len(request.AuthType) > maxAPIProbeAuth || len(request.AuthHeader) > maxAPIProbeHeader || len(request.CredentialRef) > maxAPIProbeCredRef {
		return errors.New("authentication fields are bounded")
	}
	return nil
}

func probeERP(r *http.Request, conn ERPConnector, ep connector.EndpointConfig, queryParams url.Values, body io.Reader) (*http.Response, error) {
	if responseConnector, ok := conn.(ERPResponseConnector); ok {
		return responseConnector.CallWithOptions(r.Context(), ep, queryParams, body, connector.CallOptions{PreserveErrorResponses: true})
	}
	return conn.Call(r.Context(), ep, queryParams, body)
}

func safeContentType(raw string) string {
	contentType := strings.TrimSpace(raw)
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		return strings.ToLower(mediaType)
	}
	return "application/octet-stream"
}

func writeAPIProbeError(w http.ResponseWriter, status int, message string) {
	code := ErrorAPIProbeFailed
	suggestion := "review the API definition and retry"
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = ErrorValidationFailed
		suggestion = "submit a bounded URL, method, and credential reference"
	case http.StatusBadGateway, http.StatusGatewayTimeout:
		code = ErrorUpstreamUnreachable
		suggestion = "check the ERP endpoint and network connectivity"
	case http.StatusServiceUnavailable:
		code = ErrorHealthCheckFailed
		suggestion = "check ERPBridge connector health and retry"
	}
	writeControlPlaneError(w, status, code, message, suggestion)
}
