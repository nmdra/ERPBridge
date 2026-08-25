package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nmdra/ERPBridge/internal/security"
)

// PluginProcessor is the external-plugin invocation seam used by the server.
type PluginProcessor interface {
	Process(context.Context, *Plugin, PluginInvocation) (*PluginResponse, error)
}

// PluginClient calls a separately running external plugin over HTTP JSON.
type PluginClient struct {
	httpClient *http.Client
	log        *slog.Logger
}

// NewPluginClient creates a client. An optional HTTP client is accepted for
// tests and custom transports; redirects are disabled on the effective client.
func NewPluginClient(clients ...*http.Client) *PluginClient {
	return NewPluginClientWithLogger(newDiscardLogger(), clients...)
}

// NewPluginClientWithLogger creates a client with safe structured logging.
func NewPluginClientWithLogger(log *slog.Logger, clients ...*http.Client) *PluginClient {
	baseClient := http.DefaultClient
	if len(clients) > 0 && clients[0] != nil {
		baseClient = clients[0]
	}
	client := *baseClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client.Jar = nil
	if log == nil {
		log = newDiscardLogger()
	}
	return &PluginClient{httpClient: &client, log: log}
}

// Process sends one bounded invocation and accepts only a result envelope.
func (c *PluginClient) Process(ctx context.Context, plugin *Plugin, invocation PluginInvocation) (*PluginResponse, error) {
	if c == nil {
		return nil, errors.New("plugin client is required")
	}
	if plugin == nil {
		return nil, errors.New("plugin is required")
	}
	start := time.Now()
	status := 0
	defer func() {
		c.logInvocation(plugin, invocation, status, time.Since(start))
	}()

	endpoint, err := plugin.processURL()
	if err != nil {
		return nil, errors.New("invalid plugin configuration")
	}
	authHeader, authValue, err := pluginAuthenticationHeader(plugin.Spec.Auth)
	if err != nil {
		return nil, err
	}
	endpointHostPort, insecureHTTPAllowed, err := security.ValidateOutboundTransport(endpoint, authValue != "")
	if err != nil {
		return nil, errors.New("credentialed plugin endpoint is not allowed")
	}
	if insecureHTTPAllowed {
		c.log.Warn("credentialed outbound HTTP is allowed for development",
			slog.String("endpoint", endpointHostPort),
		)
	}

	if invocation.ProtocolVersion == "" {
		invocation.ProtocolVersion = PluginProtocolVersion
	} else if invocation.ProtocolVersion != PluginProtocolVersion {
		return nil, errors.New("unsupported plugin protocol version")
	}
	if invocation.InvocationID == "" {
		invocation.InvocationID, err = newInvocationID()
		if err != nil {
			return nil, errors.New("generate plugin invocation id")
		}
	}
	if err := invocation.Validate(); err != nil {
		return nil, errors.New("invalid plugin invocation")
	}
	requestBody, err := json.Marshal(invocation)
	if err != nil {
		return nil, errors.New("encode plugin request")
	}
	if len(requestBody) > maxPluginJSONBytes {
		return nil, errors.New("plugin request exceeds maximum size")
	}

	timeout := time.Duration(plugin.Spec.TimeoutMilliseconds) * time.Millisecond
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint.String(), bytes.NewReader(requestBody))
	if err != nil {
		return nil, errors.New("create plugin request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if authHeader != "" {
		req.Header.Set(authHeader, authValue)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestContext.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("plugin request timed out")
		}
		return nil, errors.New("plugin request failed")
	}
	defer closePluginBody(resp.Body)

	status = resp.StatusCode
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("plugin returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginJSONBytes+1))
	if err != nil {
		return nil, errors.New("read plugin response")
	}
	if len(body) > maxPluginJSONBytes {
		return nil, errors.New("plugin response exceeds maximum size")
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.New("decode plugin response")
	}
	if envelope.Result == nil {
		return nil, errors.New("plugin response is missing result")
	}

	var result any
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, errors.New("decode plugin result")
	}
	return &PluginResponse{Result: result}, nil
}

func pluginAuthenticationHeader(auth *PluginAuth) (string, string, error) {
	if auth == nil {
		return "", "", nil
	}

	credential, err := resolveCredential(auth.CredentialRef)
	if err != nil {
		return "", "", errors.New("plugin credential is not configured")
	}

	switch auth.Type {
	case PluginAuthTypeBearer:
		return pluginAuthorizationHeader, "Bearer " + credential, nil
	case PluginAuthTypeAPIKey:
		header := auth.Header
		if header == "" {
			header = pluginDefaultAPIKeyHeader
		}
		return header, credential, nil
	default:
		return "", "", errors.New("invalid plugin configuration")
	}
}

func newInvocationID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func closePluginBody(body io.Closer) {
	if err := body.Close(); err != nil {
		return
	}
}

func (c *PluginClient) logInvocation(plugin *Plugin, invocation PluginInvocation, status int, duration time.Duration) {
	if c.log == nil {
		return
	}
	attrs := []any{
		slog.String("plugin_name", plugin.Metadata.Name),
		slog.String("plugin_version", plugin.Metadata.Version),
		slog.String("tool_name", invocation.Tool.Name),
		slog.String("tool_version", invocation.Tool.Version),
		slog.Int("status", status),
		slog.Duration("duration", duration),
	}
	c.log.Info("external plugin invocation completed", attrs...)
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
