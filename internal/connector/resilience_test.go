package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
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
