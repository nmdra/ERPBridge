// Package connector provides an HTTP client with built-in resilience
// features like retries and circuit breaking for communicating with ERP systems.
package connector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/metrics"
	"github.com/sony/gobreaker"
)

// AuthConfig defines the authentication credentials for an ERP request.
type AuthConfig struct {
	// Type of authentication: "api-key", "basic", or "bearer".
	Type string
	// Key is the resolved secret value.
	Key string
}

// EndpointConfig describes the target ERP API endpoint and its requirements.
type EndpointConfig struct {
	Method  string
	Path    string
	BaseURL string
	Auth    AuthConfig
}

// Client is a resilient HTTP client for ERP communication.
type Client struct {
	http *http.Client
	cb   *gobreaker.CircuitBreaker
	log  *slog.Logger
}

// NewClient creates a new resilient ERP client with the provided root logger.
func NewClient(rootLog *slog.Logger) *Client {
	log := logger.Component(rootLog, "connector")

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "ERPConnector",
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed",
				slog.String("name", name),
				slog.String("from", from.String()),
				slog.String("to", to.String()),
			)
		},
	})

	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
		cb:   cb,
		log:  log,
	}
}

func endpointIdentity(target string) string {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}

func (c *Client) applyAuth(req *http.Request, ep EndpointConfig) {
	if ep.Auth.Key == "" {
		return
	}

	switch ep.Auth.Type {
	case "api-key":
		// For Frappe/ERPNext, this is typically "token {key}:{secret}"
		// We expect the resolved Key to contain the full header value.
		req.Header.Set("Authorization", ep.Auth.Key)
	case "basic":
		// Expects Key to be base64 encoded "user:pass"
		req.Header.Set("Authorization", "Basic "+ep.Auth.Key)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+ep.Auth.Key)
	}
}

// Call executes an outbound request to the ERP endpoint with retries and circuit breaking.
func (c *Client) Call(ctx context.Context, ep EndpointConfig, queryParams url.Values, body io.Reader) (*http.Response, error) {
	start := time.Now()
	log := logger.FromContext(ctx)

	target := ep.BaseURL + ep.Path
	if len(queryParams) > 0 {
		target += "?" + queryParams.Encode()
	}

	endpoint := endpointIdentity(target)

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = io.ReadAll(body)
	}

	res, err := c.cb.Execute(func() (any, error) {
		var lastResp *http.Response
		err := retry.Do(
			func() error {
				var currentBody io.Reader
				if bodyBytes != nil {
					currentBody = bytes.NewReader(bodyBytes)
				}

				req, err := http.NewRequestWithContext(ctx, ep.Method, target, currentBody)
				if err != nil {
					return retry.Unrecoverable(fmt.Errorf("build request: %w", err))
				}

				c.applyAuth(req, ep)
				req.Header.Set("Content-Type", "application/json")

				log.Debug("erp request attempt",
					slog.String("method", ep.Method),
					slog.String("endpoint", endpoint),
				)

				resp, err := c.http.Do(req)
				if err != nil {
					return err // Retry on network error
				}

				if resp.StatusCode == http.StatusTooManyRequests ||
					resp.StatusCode >= 500 {
					_ = resp.Body.Close()
					return fmt.Errorf("transient erp error: %d", resp.StatusCode)
				}

				lastResp = resp
				return nil
			},
			retry.Context(ctx),
			retry.Attempts(3),
			retry.Delay(500*time.Millisecond),
			retry.MaxJitter(100*time.Millisecond),
			retry.LastErrorOnly(true),
		)
		return lastResp, err
	})

	if err != nil {
		log.Error("erp request failed",
			slog.String("endpoint", endpoint),
			slog.String("error_type", fmt.Sprintf("%T", err)),
		)
		return nil, err
	}

	resp := res.(*http.Response)
	duration := time.Since(start)
	latency := int(duration.Milliseconds())

	// Record metrics
	metrics.ERPRequestsTotal.WithLabelValues(ep.Method, endpoint, fmt.Sprintf("%d", resp.StatusCode)).Inc()
	metrics.ERPLatency.WithLabelValues(ep.Method, endpoint).Observe(duration.Seconds())

	log.Info("erp response",
		slog.String("endpoint", endpoint),
		slog.Int("status_code", resp.StatusCode),
		slog.Int("latency_ms", latency),
	)

	return resp, nil
}
