package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nmdra/ERPBridge/internal/connector"
)

var (
	// ErrRevisionInactive indicates that withdrawal won the dispatch-commitment race.
	ErrRevisionInactive = errors.New("selected tool revision is no longer active")
	// ErrOriginDenied indicates that the final connector endpoint has an unapproved origin.
	ErrOriginDenied = errors.New("effective ERP origin is not approved")
)

// AuthorityEvent is a sanitized ordering record. It contains no arguments,
// credentials, request headers, paths, or ERP response data.
type AuthorityEvent struct {
	Sequence       uint64 `json:"sequence"`
	Type           string `json:"type"`
	ToolName       string `json:"toolName"`
	ToolVersion    string `json:"toolVersion"`
	ResourceDigest string `json:"resourceDigest,omitempty"`
	At             string `json:"at"`
}

// AuthorityHooks provides acknowledged test/evaluation barriers at the final
// connector boundary. Production callers normally leave it nil.
type AuthorityHooks struct {
	BeforeCommit func(AuthorityEvent)
	OnEvent      func(AuthorityEvent)
}

type authorityConnector struct {
	server *Server
	tool   *Tool
	next   ERPConnector
}

type authorityResponseConnector struct {
	*authorityConnector
	nextResponse ERPResponseConnector
}

func (s *Server) authorizedERPConnector(tool *Tool) ERPConnector {
	base := &authorityConnector{server: s, tool: tool, next: s.connector}
	if responseConnector, ok := s.connector.(ERPResponseConnector); ok {
		return &authorityResponseConnector{authorityConnector: base, nextResponse: responseConnector}
	}
	return base
}

func (c *authorityConnector) Call(ctx context.Context, endpoint connector.EndpointConfig, query url.Values, body io.Reader) (*http.Response, error) {
	return c.call(ctx, endpoint, query, body, connector.CallOptions{}, nil)
}

func (c *authorityResponseConnector) CallWithOptions(ctx context.Context, endpoint connector.EndpointConfig, query url.Values, body io.Reader, options connector.CallOptions) (*http.Response, error) {
	return c.call(ctx, endpoint, query, body, options, c.nextResponse)
}

func (c *authorityConnector) call(ctx context.Context, endpoint connector.EndpointConfig, query url.Values, body io.Reader, options connector.CallOptions, responseConnector ERPResponseConnector) (*http.Response, error) {
	if err := validateEffectiveOrigin(c.tool, endpoint); err != nil {
		c.server.emitAuthorityEvent("origin_denied", c.tool)
		return nil, err
	}

	barrier := c.server.nextAuthorityEvent("before_commit", c.tool)
	if hooks := c.server.authorityHooks(); hooks != nil {
		if hooks.OnEvent != nil {
			hooks.OnEvent(barrier)
		}
		if hooks.BeforeCommit != nil {
			hooks.BeforeCommit(barrier)
		}
	}

	var event AuthorityEvent
	var response *http.Response
	var callErr error
	func() {
		c.server.authorityMu.RLock()
		defer c.server.authorityMu.RUnlock()
		if !c.server.revisionActive(c.tool) {
			event = c.server.nextAuthorityEvent("commit_denied", c.tool)
			callErr = ErrRevisionInactive
			return
		}
		event = c.server.nextAuthorityEvent("dispatch_committed", c.tool)
		if responseConnector != nil {
			// Origin authority is bound to the endpoint validated above. Returning
			// the first redirect prevents entry into an unapproved second origin.
			options.DisableRedirects = true
			response, callErr = responseConnector.CallWithOptions(ctx, endpoint, query, body, options)
			return
		}
		response, callErr = c.next.Call(ctx, endpoint, query, body)
	}()
	// Hooks run outside authorityMu so evaluation observers can safely perform
	// follow-up control-plane operations. Sequence records commitment order.
	c.server.deliverAuthorityEvent(event)
	return response, callErr
}

func bindApprovedOrigins(tool *Tool) error {
	if tool.Spec.Execution.Endpoint == "" {
		return nil
	}
	origins := tool.Spec.Execution.ApprovedOrigins
	if len(origins) == 0 {
		origin, err := normalizedOrigin(resolveEffectiveEndpoint(tool.Spec.Execution.Endpoint, os.Getenv("ERP_BASE_URL")))
		if err != nil {
			return fmt.Errorf("derive approved origin: %w", err)
		}
		origins = []string{origin}
	}

	normalized := make([]string, 0, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		origin, err := normalizedOrigin(raw)
		if err != nil {
			return fmt.Errorf("invalid approved origin: %w", err)
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		normalized = append(normalized, origin)
	}
	sort.Strings(normalized)
	tool.Spec.Execution.ApprovedOrigins = normalized
	return nil
}

func validateEffectiveOrigin(tool *Tool, endpoint connector.EndpointConfig) error {
	effective := endpoint.Path
	if endpoint.BaseURL != "" {
		effective = strings.TrimSuffix(endpoint.BaseURL, "/") + "/" + strings.TrimPrefix(endpoint.Path, "/")
	}
	origin, err := normalizedOrigin(effective)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrOriginDenied, err.Error())
	}
	for _, approved := range tool.Spec.Execution.ApprovedOrigins {
		normalized, normalizeErr := normalizedOrigin(approved)
		if normalizeErr == nil && normalized == origin {
			return nil
		}
	}
	return fmt.Errorf("%w", ErrOriginDenied)
}

func normalizedOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("absolute HTTP origin is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported origin scheme")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("userinfo is not allowed in an origin")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func (s *Server) authorityHooks() *AuthorityHooks {
	s.authorityEventMu.Lock()
	defer s.authorityEventMu.Unlock()
	return s.AuthorityHooks
}

func (s *Server) nextAuthorityEvent(eventType string, tool *Tool) AuthorityEvent {
	s.authorityEventMu.Lock()
	defer s.authorityEventMu.Unlock()
	s.authoritySequence++
	return AuthorityEvent{
		Sequence:       s.authoritySequence,
		Type:           eventType,
		ToolName:       tool.Metadata.Name,
		ToolVersion:    tool.Metadata.Version,
		ResourceDigest: tool.Metadata.ResourceDigest,
		At:             time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (s *Server) emitAuthorityEvent(eventType string, tool *Tool) {
	s.deliverAuthorityEvent(s.nextAuthorityEvent(eventType, tool))
}

func (s *Server) deliverAuthorityEvent(event AuthorityEvent) {
	hooks := s.authorityHooks()
	if hooks != nil && hooks.OnEvent != nil {
		hooks.OnEvent(event)
	}
}

func (s *Server) revisionActive(tool *Tool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active, err := s.registry.Resolve(tool.Metadata.Name, tool.Metadata.Version)
	if err != nil {
		return false
	}
	if tool.Metadata.ResourceDigest == "" {
		return true
	}
	return active.Metadata.ResourceDigest == tool.Metadata.ResourceDigest
}

var _ ERPResponseConnector = (*authorityResponseConnector)(nil)
