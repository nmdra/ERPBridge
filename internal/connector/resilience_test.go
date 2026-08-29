package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Call_Retry(t *testing.T) {
	log := logger.Init()
	client := NewClient(log)

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ep := EndpointConfig{
		Method:  http.MethodGet,
		Path:    "",
		BaseURL: server.URL,
	}

	resp, err := client.Call(context.Background(), ep, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestClient_Call_DoesNotRetryPermanentFailures(t *testing.T) {
	client := NewClient(logger.Init())
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	resp, err := client.Call(context.Background(), EndpointConfig{Method: http.MethodGet, BaseURL: server.URL}, nil, nil)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, int32(1), attempts.Load())
	require.NoError(t, resp.Body.Close())
}

func TestClient_Call_DoesNotRetrySideEffectingRequests(t *testing.T) {
	client := NewClient(logger.Init())
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resp, err := client.CallWithOptions(context.Background(), EndpointConfig{
		Method:  http.MethodPost,
		BaseURL: server.URL,
	}, nil, nil, CallOptions{PreserveErrorResponses: true})

	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Equal(t, int32(1), attempts.Load())
	require.NoError(t, resp.Body.Close())
}

func TestClient_Call_HonorsRetryAfterZero(t *testing.T) {
	client := NewClient(logger.Init())
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	start := time.Now()
	resp, err := client.Call(context.Background(), EndpointConfig{Method: http.MethodGet, BaseURL: server.URL}, nil, nil)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(2), attempts.Load())
	require.Less(t, time.Since(start), 400*time.Millisecond)
	require.NoError(t, resp.Body.Close())
}

func TestParseRetryAfterSupportsSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)

	delay, ok := parseRetryAfter("3", now)
	require.True(t, ok)
	require.Equal(t, 3*time.Second, delay)

	delay, ok = parseRetryAfter(now.Add(4*time.Second).Format(http.TimeFormat), now)
	require.True(t, ok)
	require.Equal(t, 4*time.Second, delay)

	_, ok = parseRetryAfter("invalid", now)
	require.False(t, ok)
}

func TestClient_Call_RetryAfterHonorsContextDeadline(t *testing.T) {
	client := NewClient(logger.Init())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.Call(ctx, EndpointConfig{Method: http.MethodGet, BaseURL: server.URL}, nil, nil)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClient_CallWithOptions_PreservesResponseAndTripsCircuitBreaker(t *testing.T) {
	log := logger.Init()
	client := NewClient(log)
	client.cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "RawResponseCB",
		MaxRequests: 1,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.TotalFailures >= 1
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"retry"}`))
	}))
	defer server.Close()

	ep := EndpointConfig{Method: http.MethodGet, BaseURL: server.URL}
	resp, err := client.CallWithOptions(context.Background(), ep, nil, nil, CallOptions{PreserveErrorResponses: true})
	assert.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.NoError(t, resp.Body.Close())

	_, err = client.CallWithOptions(context.Background(), ep, nil, nil, CallOptions{PreserveErrorResponses: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
}

func TestClient_Call_CircuitBreaker(t *testing.T) {
	log := logger.Init()
	client := NewClient(log)

	// Configure CB to trip after 1 failure
	client.cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "TestCB",
		MaxRequests: 1,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.TotalFailures >= 1
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ep := EndpointConfig{
		Method:  http.MethodGet,
		Path:    "",
		BaseURL: server.URL,
	}

	// 1st failure (and all 3 retries) -> should trip CB
	_, err := client.Call(context.Background(), ep, nil, nil)
	assert.Error(t, err)

	// next call -> should be fast-fail by CB
	_, err = client.Call(context.Background(), ep, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
}
