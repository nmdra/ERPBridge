package mcp

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/connector"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

type recordingERPConnector struct {
	mu      sync.Mutex
	entries int
	entered chan struct{}
	release chan struct{}
}

func (c *recordingERPConnector) Call(context.Context, connector.EndpointConfig, url.Values, io.Reader) (*http.Response, error) {
	c.mu.Lock()
	c.entries++
	entered := c.entered
	release := c.release
	c.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if release != nil {
		<-release
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(newEmptyJSONReader())}, nil
}

func (c *recordingERPConnector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries
}

func newEmptyJSONReader() io.Reader {
	return &staticReader{data: []byte(`{"ok":true}`)}
}

type staticReader struct {
	data []byte
}

func (r *staticReader) Read(target []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(target, r.data)
	r.data = r.data[n:]
	return n, nil
}

func authorityTestTool(name string, approvedOrigins []string) *Tool {
	return &Tool{
		Metadata: Metadata{Name: name, Version: "1.0.0", IsActive: true, IsServing: true, ResourceDigest: "digest-" + name},
		Spec: ToolSpec{
			Description: Description{Short: "authority test"},
			Execution:   Execution{Type: "http", Method: http.MethodGet, Endpoint: "http://approved.example/items", ApprovedOrigins: approvedOrigins},
		},
	}
}

func TestWithdrawalBeforeDispatchCommitProducesZeroConnectorEntries(t *testing.T) {
	connectorSpy := &recordingERPConnector{}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := authorityTestTool("withdrawal-test", []string{"http://approved.example"})
	s.RegisterTool(tool)

	acknowledged := make(chan struct{})
	release := make(chan struct{})
	var eventsMu sync.Mutex
	var events []AuthorityEvent
	s.AuthorityHooks = &AuthorityHooks{
		BeforeCommit: func(AuthorityEvent) {
			close(acknowledged)
			<-release
		},
		OnEvent: func(event AuthorityEvent) {
			eventsMu.Lock()
			events = append(events, event)
			eventsMu.Unlock()
		},
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	completed := make(chan error, 1)
	go func() {
		_, err := s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
		completed <- err
	}()
	<-acknowledged
	s.DeregisterTool(tool.Metadata.Name, tool.Metadata.Version)
	close(release)
	require.NoError(t, <-completed, "execution errors are returned as MCP isError results")
	require.Zero(t, connectorSpy.count())

	eventsMu.Lock()
	defer eventsMu.Unlock()
	require.Len(t, events, 3)
	require.Equal(t, "before_commit", events[0].Type)
	require.Equal(t, "withdrawal_committed", events[1].Type)
	require.Equal(t, "commit_denied", events[2].Type)
	require.Less(t, events[0].Sequence, events[1].Sequence)
	require.Less(t, events[1].Sequence, events[2].Sequence)
}

func TestDispatchCommitBeforeWithdrawalMayComplete(t *testing.T) {
	connectorSpy := &recordingERPConnector{entered: make(chan struct{}), release: make(chan struct{})}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := authorityTestTool("committed-test", []string{"http://approved.example"})
	s.RegisterTool(tool)

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	completed := make(chan *mcp.CallToolResult, 1)
	go func() {
		result, _ := s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
		completed <- result
	}()
	<-connectorSpy.entered

	withdrawn := make(chan struct{})
	go func() {
		s.DeregisterTool(tool.Metadata.Name, tool.Metadata.Version)
		close(withdrawn)
	}()
	close(connectorSpy.release)
	result := <-completed
	require.False(t, result.IsError)
	<-withdrawn
	require.Equal(t, 1, connectorSpy.count())
}

func TestEffectiveOriginIsCheckedImmediatelyBeforeConnectorEntry(t *testing.T) {
	connectorSpy := &recordingERPConnector{}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	denied := authorityTestTool("origin-denied", []string{"https://different.example"})
	s.RegisterTool(denied)
	request := mcp.CallToolRequest{}
	request.Params.Name = denied.Metadata.Name
	request.Params.Arguments = map[string]any{}
	result, err := s.handleBoundMCPToolCall(denied.Metadata.Name)(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Zero(t, connectorSpy.count())

	allowed := authorityTestTool("origin-allowed", []string{"http://approved.example"})
	s.RegisterTool(allowed)
	request.Params.Name = allowed.Metadata.Name
	result, err = s.handleBoundMCPToolCall(allowed.Metadata.Name)(context.Background(), request)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, 1, connectorSpy.count())
}

func TestRuntimeBaseURLChangeCannotEscapeAdmittedOrigin(t *testing.T) {
	t.Setenv("ERP_BASE_URL", "http://approved.example")
	connectorSpy := &recordingERPConnector{}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := authorityTestTool("rewrite-origin", nil)
	tool.Spec.Execution.Endpoint = "/items"
	s.RegisterTool(tool)
	require.Equal(t, []string{"http://approved.example"}, tool.Spec.Execution.ApprovedOrigins)

	t.Setenv("ERP_BASE_URL", "http://unapproved.example")
	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	result, err := s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Zero(t, connectorSpy.count())
}

func TestMCPConnectorBoundaryDisablesRedirects(t *testing.T) {
	var observed connector.CallOptions
	connectorSpy := &MockConnector{CallWithOptionsFunc: func(_ context.Context, _ connector.EndpointConfig, _ url.Values, _ io.Reader, options connector.CallOptions) (*http.Response, error) {
		observed = options
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(newEmptyJSONReader())}, nil
	}}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := authorityTestTool("redirect-boundary", []string{"http://approved.example"})
	s.RegisterTool(tool)

	response := s.MCPServer().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"redirect-boundary","arguments":{}}}`))
	rpcResponse, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)
	result, ok := rpcResponse.Result.(*mcp.CallToolResult)
	require.True(t, ok)
	require.False(t, result.IsError)
	require.True(t, observed.DisableRedirects)
}

func TestAuthorityEventCallbackCanWithdrawWithoutDeadlock(t *testing.T) {
	connectorSpy := &recordingERPConnector{}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := authorityTestTool("event-reentry", []string{"http://approved.example"})
	s.RegisterTool(tool)
	withdrawn := make(chan struct{})
	s.AuthorityHooks = &AuthorityHooks{OnEvent: func(event AuthorityEvent) {
		if event.Type == "dispatch_committed" {
			s.DeregisterTool(tool.Metadata.Name, tool.Metadata.Version)
			close(withdrawn)
		}
	}}

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	completed := make(chan struct{})
	go func() {
		_, _ = s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
		close(completed)
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("authority event callback deadlocked during withdrawal")
	}
	select {
	case <-withdrawn:
	default:
		t.Fatal("authority callback did not withdraw the revision")
	}
}
