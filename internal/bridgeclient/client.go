// Package bridgeclient provides the shared, authenticated HTTP seams used by
// bridgectl and the local ERPBridge Console.
package bridgeclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/nmdra/ERPBridge/internal/config"
)

const defaultMaxResponseBytes int64 = 4 << 20

var (
	// ErrResponseTooLarge indicates that an upstream response exceeded its limit.
	ErrResponseTooLarge = errors.New("upstream response exceeds size limit")
	// ErrTargetUnavailable indicates that the selected context has no target URL.
	ErrTargetUnavailable = errors.New("upstream target is unavailable")
)

// Target selects one of the two configured ERPBridge endpoints.
type Target string

const (
	// TargetServer selects the management endpoint.
	TargetServer Target = "server"
	// TargetMCPServer selects the MCP endpoint.
	TargetMCPServer Target = "mcp-server"
)

// Client sends requests to fixed paths on a configured ERPBridge context.
type Client struct {
	serverURL        *url.URL
	mcpServerURL     *url.URL
	token            string
	httpClient       *http.Client
	MaxResponseBytes int64
}

// New creates a client and validates every configured endpoint that is set.
func New(ctx config.Context, tokenOverride string) (*Client, error) {
	serverURL, err := parseServerURL(ctx.Server)
	if err != nil {
		return nil, fmt.Errorf("validate Server URL: %w", err)
	}
	mcpURL, err := parseServerURL(ctx.MCPServer)
	if err != nil {
		return nil, fmt.Errorf("validate MCPServer URL: %w", err)
	}
	return &Client{
		serverURL:    serverURL,
		mcpServerURL: mcpURL,
		token:        ResolveToken(tokenOverride, os.Getenv("BRIDGE_API_TOKEN"), ctx.APIToken),
		httpClient: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		MaxResponseBytes: defaultMaxResponseBytes,
	}, nil
}

// Do sends one request to a fixed relative path. Browser headers are ignored
// except for the small content-negotiation allow-list.
func (c *Client) Do(ctx context.Context, target Target, method, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	base, err := c.baseURL(target)
	if err != nil {
		return nil, err
	}
	relative, err := parseRelativePath(path)
	if err != nil {
		return nil, err
	}
	requestURL := *base
	requestURL.Path = strings.TrimRight(base.Path, "/") + relative.Path
	requestURL.RawPath = ""
	requestURL.RawQuery = relative.RawQuery
	requestURL.Fragment = ""

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}
	copyAllowedHeaders(request.Header, headers)
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	response.Body = &boundedBody{body: response.Body, max: c.MaxResponseBytes}
	return response, nil
}

// NewAuthenticatedRequest creates the request seam shared with existing CLI
// commands. It preserves the CLI's explicit-token precedence behavior.
func NewAuthenticatedRequest(ctx context.Context, method, target string, body io.Reader, token string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, nil
}

// ResolveToken returns the first configured token in precedence order.
func ResolveToken(explicit, environment, contextToken string) string {
	if explicit != "" {
		return explicit
	}
	if environment != "" {
		return environment
	}
	return contextToken
}

// ValidateServerURL validates a configured ERPBridge base URL.
func ValidateServerURL(raw string) error {
	if raw == "" {
		return errors.New("URL is empty")
	}
	_, err := parseServerURL(raw)
	return err
}

func (c *Client) baseURL(target Target) (*url.URL, error) {
	switch target {
	case TargetServer:
		if c.serverURL == nil {
			return nil, ErrTargetUnavailable
		}
		return c.serverURL, nil
	case TargetMCPServer:
		if c.mcpServerURL == nil {
			return nil, ErrTargetUnavailable
		}
		return c.mcpServerURL, nil
	default:
		return nil, fmt.Errorf("unknown upstream target %q", target)
	}
}

func parseServerURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("URL must use http or https and include a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("URL must not contain userinfo, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func parseRelativePath(raw string) (*url.URL, error) {
	if raw == "" || !strings.HasPrefix(raw, "/") {
		return nil, errors.New("upstream path must be absolute and relative to the configured base")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse upstream path: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("upstream path must not contain a destination or fragment")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == ".." {
			return nil, errors.New("upstream path must not escape its base")
		}
	}
	return parsed, nil
}

func copyAllowedHeaders(destination, source http.Header) {
	for _, key := range []string{"Accept", "Content-Type"} {
		for _, value := range source.Values(key) {
			destination.Add(key, value)
		}
	}
}

type boundedBody struct {
	body   io.ReadCloser
	read   int64
	max    int64
	failed bool
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.failed {
		return 0, ErrResponseTooLarge
	}
	if b.max <= 0 {
		return b.body.Read(p)
	}
	n, err := b.body.Read(p)
	b.read += int64(n)
	if b.read > b.max {
		b.failed = true
		return n, ErrResponseTooLarge
	}
	return n, err
}

func (b *boundedBody) Close() error { return b.body.Close() }
