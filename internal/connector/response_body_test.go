package connector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Hold the body until Call has returned, so buffered localhost responses cannot
// hide premature cancellation after headers arrive.
func streamingResponseFixture(t *testing.T, status int) (string, func(), *atomic.Int32) {
	t.Helper()
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, `{"decision":"deny","reason":"fixture policy"}`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		unblock()
		server.Close()
	})
	return server.URL, unblock, calls
}

func TestClientStreamingResponseBodySurvivesCallReturn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		preserve bool
	}{
		{name: "success", status: http.StatusOK},
		{name: "success_with_raw_capture", status: http.StatusOK, preserve: true},
		{name: "preserved_error", status: http.StatusServiceUnavailable, preserve: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, release, calls := streamingResponseFixture(t, tc.status)
			resp, err := NewClient(slog.Default()).CallWithOptions(context.Background(), EndpointConfig{
				Method: http.MethodPost, BaseURL: endpoint,
			}, nil, nil, CallOptions{PreserveErrorResponses: tc.preserve})
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, tc.status, resp.StatusCode)
			require.NoError(t, resp.Request.Context().Err(), "Call must not cancel an unread response")

			release()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"decision":"deny","reason":"fixture policy"}`, string(body))
			require.EqualValues(t, 1, calls.Load(), "POST must not be replayed, including preserved 503 responses")
			require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled, "EOF must release the connector deadline")
		})
	}
}

func TestClientResponseBodyCloseReleasesContext(t *testing.T) {
	endpoint, _, calls := streamingResponseFixture(t, http.StatusOK)
	client := NewClient(slog.Default())
	// Inspect the connector's 30-second deadline independently of http.Client's
	// shorter 15-second timeout, which has its own regression below.
	client.http.Timeout = 0
	resp, err := client.Call(context.Background(), EndpointConfig{
		Method: http.MethodGet, BaseURL: endpoint,
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.NoError(t, resp.Request.Context().Err())
	deadline, ok := resp.Request.Context().Deadline()
	require.True(t, ok, "the overall connector deadline must still cover the body")
	require.InDelta(t, maxCallDuration.Seconds(), time.Until(deadline).Seconds(), 2)

	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
	require.NoError(t, resp.Body.Close(), "repeated cleanup must be safe")
	require.EqualValues(t, 1, calls.Load())
}

func TestClientResponseBodyParentCancellationStillStopsRead(t *testing.T) {
	endpoint, _, calls := streamingResponseFixture(t, http.StatusOK)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := NewClient(slog.Default()).Call(ctx, EndpointConfig{
		Method: http.MethodPost, BaseURL: endpoint,
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.NoError(t, resp.Request.Context().Err())

	readDone := make(chan error, 1)
	go func() { _, readErr := io.ReadAll(resp.Body); readDone <- readErr }()
	cancel()
	select {
	case readErr := <-readDone:
		require.ErrorIs(t, readErr, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("response read ignored parent cancellation")
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestClientResponseBodyParentDeadlineStillStopsRead(t *testing.T) {
	endpoint, _, calls := streamingResponseFixture(t, http.StatusOK)
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	resp, err := NewClient(slog.Default()).Call(ctx, EndpointConfig{
		Method: http.MethodPost, BaseURL: endpoint,
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.NoError(t, resp.Request.Context().Err())
	parentDeadline, _ := ctx.Deadline()
	responseDeadline, ok := resp.Request.Context().Deadline()
	require.True(t, ok)
	require.Equal(t, parentDeadline, responseDeadline, "body ownership must not reset the deadline")

	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, calls.Load())
}

func TestClientResponseBodyHTTPTimeoutStillStopsRead(t *testing.T) {
	endpoint, _, calls := streamingResponseFixture(t, http.StatusOK)
	client := NewClient(slog.Default())
	client.http.Timeout = 350 * time.Millisecond
	resp, err := client.Call(context.Background(), EndpointConfig{
		Method: http.MethodPost, BaseURL: endpoint,
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.NoError(t, resp.Request.Context().Err())

	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, resp.Request.Context().Err(), context.DeadlineExceeded, "the HTTP client deadline must remain effective")
	require.EqualValues(t, 1, calls.Load())
}

func TestClientResponseBodyTruncatedReadReleasesContext(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, `{"cut":`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(unblock)
	resp, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
		Method: http.MethodGet, BaseURL: server.URL,
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.NoError(t, resp.Request.Context().Err())
	unblock()
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
}

type responseBodyRoundTripper func(*http.Request) (*http.Response, error)

func (f responseBodyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientResponseBodyFailedCallReleasesContext(t *testing.T) {
	client := NewClient(slog.Default())
	var callCtx context.Context
	calls := 0
	client.http.Transport = responseBodyRoundTripper(func(r *http.Request) (*http.Response, error) {
		callCtx = r.Context()
		calls++
		return nil, errors.New("fixture transport failure")
	})
	resp, err := client.Call(context.Background(), EndpointConfig{
		Method: http.MethodPost, BaseURL: "http://fixture.invalid",
	}, nil, nil)
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, 1, calls)
	require.ErrorIs(t, callCtx.Err(), context.Canceled)
}

func TestClientResponseBodyEmptyResponseReleasesContext(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			resp, err := NewClient(slog.Default()).Call(context.Background(), EndpointConfig{
				Method: method, BaseURL: server.URL,
			}, nil, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Empty(t, body)
			require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
		})
	}
}
