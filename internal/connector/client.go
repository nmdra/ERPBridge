// Package connector provides an HTTP client with built-in resilience
// features like retries and circuit breaking for communicating with ERP systems.
package connector

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/nmdra/ERPBridge/internal/faults"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/nmdra/ERPBridge/internal/metrics"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/sony/gobreaker"
)

const (
	authTypeBearer   = "bearer"
	maxRetryAttempts = 3
	retryDelayBase   = 500 * time.Millisecond
	retryDelayJitter = 100 * time.Millisecond
	maxRetryAfter    = 30 * time.Second
	maxCallDuration  = 30 * time.Second
)

func retrySafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

type retryAttemptError struct {
	err           error
	statusCode    int
	retryAfter    time.Duration
	hasRetryAfter bool
}

func (e *retryAttemptError) Error() string { return e.err.Error() }
func (e *retryAttemptError) Unwrap() error { return e.err }

// ParseRetryAfter parses either the delta-seconds or HTTP-date form of the
// standard Retry-After response header.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return boundedRetryAfter(when.Sub(now)), true
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	return ParseRetryAfter(value, now)
}

func boundedRetryAfter(delay time.Duration) time.Duration {
	if delay < 0 {
		return 0
	}
	if delay > maxRetryAfter {
		return maxRetryAfter
	}
	return delay
}

func retryDelay(_ uint, err error, _ *retry.Config) time.Duration {
	var attemptErr *retryAttemptError
	if errors.As(err, &attemptErr) && attemptErr.hasRetryAfter {
		return boundedRetryAfter(attemptErr.retryAfter)
	}
	jitter, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(retryDelayJitter)+1))
	if err != nil {
		return retryDelayBase
	}
	return retryDelayBase + time.Duration(jitter.Int64())
}

func faultKind(err error) faults.Kind {
	if fault, ok := faults.As(err); ok {
		return fault.Kind
	}
	return faults.KindInternal
}

func classifyCallError(err error) error {
	if err == nil {
		return nil
	}
	var attemptErr *retryAttemptError
	cause := err
	retryAfter := time.Duration(0)
	if errors.As(err, &attemptErr) && attemptErr != nil {
		cause = attemptErr.err
		if attemptErr.hasRetryAfter {
			retryAfter = boundedRetryAfter(attemptErr.retryAfter)
		}
		if attemptErr.statusCode == http.StatusTooManyRequests {
			message := "the ERP service is temporarily rate limited; retry later"
			if retryAfter > 0 {
				message = fmt.Sprintf("the ERP service is temporarily rate limited; retry after %d seconds", int64((retryAfter+time.Second-1)/time.Second))
			}
			return faults.New(faults.KindRateLimited, message, true, retryAfter, cause)
		}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return faults.New(faults.KindDependencyTimeout, "the ERP service timed out; retry later", true, 0, cause)
	}
	if errors.Is(cause, context.Canceled) {
		return faults.New(faults.KindDependencyTimeout, "the ERP request was canceled", false, 0, cause)
	}
	var netErr net.Error
	if errors.As(cause, &netErr) && netErr.Timeout() {
		return faults.New(faults.KindDependencyTimeout, "the ERP service timed out; retry later", true, 0, cause)
	}
	if errors.Is(cause, gobreaker.ErrOpenState) {
		return faults.New(faults.KindDependencyUnavailable, "the ERP dependency circuit breaker is open; retry later", true, 0, cause)
	}
	if errors.Is(cause, gobreaker.ErrTooManyRequests) {
		return faults.New(faults.KindDependencyUnavailable, "the ERP dependency is busy; retry later", true, 0, cause)
	}
	return faults.New(faults.KindDependencyUnavailable, "the ERP service is unavailable; retry later", true, 0, cause)
}

// AuthConfig defines the authentication credentials for an ERP request.
type AuthConfig struct {
	// Type of authentication: "api-key", "basic", or "bearer".
	Type string
	// Key is the resolved secret value.
	Key string
	// Header overrides the default Authorization header for this request.
	Header string
}

// EndpointConfig describes the target ERP API endpoint and its requirements.
type EndpointConfig struct {
	Method  string
	Path    string
	BaseURL string
	Headers map[string]string
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

func (c *Client) clientForRequest(credentialPresent bool) *http.Client {
	if !credentialPresent {
		return c.http
	}

	client := *c.http
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

func isProtectedGeneratedHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "proxy-authorization", "cookie", "host", "connection", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade", "content-length", "content-type":
		return true
	default:
		return false
	}
}

func applyGeneratedHeaders(req *http.Request, headers map[string]string) {
	for name, value := range headers {
		if name == "" || isProtectedGeneratedHeader(name) {
			continue
		}
		req.Header.Set(name, value)
	}
}

func (c *Client) applyAuth(req *http.Request, ep EndpointConfig) {
	if ep.Auth.Key == "" {
		return
	}

	header := ep.Auth.Header
	if header == "" {
		header = "Authorization"
	}
	value := ep.Auth.Key
	switch ep.Auth.Type {
	case "api-key":
		// For Frappe/ERPNext, this is typically "token {key}:{secret}"
		// and the resolved Key contains the full header value.
	case "basic":
		// Expects Key to be base64 encoded "user:pass".
		value = "Basic " + ep.Auth.Key
	case authTypeBearer:
		value = "Bearer " + ep.Auth.Key
	}
	req.Header.Set(header, value)
}

// CallOptions controls optional response handling without changing the
// default connector behavior.
type CallOptions struct {
	// PreserveErrorResponses retains the final 429 or 5xx response after the
	// normal retry budget while still recording the failure in the circuit breaker.
	PreserveErrorResponses bool
	// DisableRedirects returns the first redirect response instead of following it.
	DisableRedirects bool
}

// preservedResponseError keeps a terminal response available to callers while
// returning an error to the circuit breaker.
type preservedResponseError struct {
	response *http.Response
	err      error
}

func (e *preservedResponseError) Error() string { return e.err.Error() }
func (e *preservedResponseError) Unwrap() error { return e.err }

// Call executes an outbound request to the ERP endpoint with retries and circuit breaking.
func (c *Client) Call(ctx context.Context, ep EndpointConfig, queryParams url.Values, body io.Reader) (*http.Response, error) {
	return c.call(ctx, ep, queryParams, body, CallOptions{})
}

// CallWithOptions executes an outbound request with explicit response handling.
func (c *Client) CallWithOptions(ctx context.Context, ep EndpointConfig, queryParams url.Values, body io.Reader, options CallOptions) (*http.Response, error) {
	return c.call(ctx, ep, queryParams, body, options)
}

func (c *Client) call(ctx context.Context, ep EndpointConfig, queryParams url.Values, body io.Reader, options CallOptions) (*http.Response, error) {
	start := time.Now()
	log := logger.FromContext(ctx)
	callCtx, cancel := context.WithTimeout(ctx, maxCallDuration)
	defer cancel()

	target := ep.BaseURL + ep.Path
	if len(queryParams) > 0 {
		target += "?" + queryParams.Encode()
	}

	endpoint := endpointIdentity(target)
	credentialPresent := ep.Auth.Key != ""
	targetURL, parseErr := url.Parse(target)
	if parseErr != nil {
		return nil, fmt.Errorf("ERP endpoint is not allowed")
	}
	allowedHost, insecureHTTPAllowed, validateErr := security.ValidateOutboundTransport(targetURL, credentialPresent)
	if validateErr != nil {
		return nil, fmt.Errorf("ERP endpoint is not allowed")
	}
	if credentialPresent && insecureHTTPAllowed {
		log.Warn("credentialed outbound HTTP is allowed for development",
			slog.String("endpoint", allowedHost),
		)
	}
	outboundClient := c.clientForRequest(credentialPresent || options.DisableRedirects)

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = io.ReadAll(body)
	}

	res, err := c.cb.Execute(func() (any, error) {
		var lastResp *http.Response
		var lastTransientResp *http.Response
		attempts := uint(1)
		if retrySafeMethod(ep.Method) {
			attempts = maxRetryAttempts
		}
		err := retry.Do(
			func() error {
				var currentBody io.Reader
				if bodyBytes != nil {
					currentBody = bytes.NewReader(bodyBytes)
				}

				req, err := http.NewRequestWithContext(callCtx, ep.Method, target, currentBody)
				if err != nil {
					return retry.Unrecoverable(fmt.Errorf("build request: %w", err))
				}

				applyGeneratedHeaders(req, ep.Headers)
				c.applyAuth(req, ep)
				req.Header.Set("Content-Type", "application/json")

				log.Debug("erp request attempt",
					slog.String("method", ep.Method),
					slog.String("endpoint", endpoint),
				)

				resp, err := outboundClient.Do(req)
				if err != nil {
					return &retryAttemptError{err: err}
				}

				if resp.StatusCode == http.StatusTooManyRequests ||
					resp.StatusCode >= 500 {
					if lastTransientResp != nil {
						_ = lastTransientResp.Body.Close()
					}
					retryAfter, hasRetryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
					lastTransientResp = resp
					return &retryAttemptError{
						err:           fmt.Errorf("transient erp error: %d", resp.StatusCode),
						statusCode:    resp.StatusCode,
						retryAfter:    retryAfter,
						hasRetryAfter: hasRetryAfter,
					}
				}

				if lastTransientResp != nil {
					_ = lastTransientResp.Body.Close()
					lastTransientResp = nil
				}
				lastResp = resp
				return nil
			},
			retry.Context(callCtx),
			retry.Attempts(attempts),
			retry.DelayType(retryDelay),
			retry.LastErrorOnly(true),
		)
		if err != nil && options.PreserveErrorResponses && lastTransientResp != nil {
			return nil, &preservedResponseError{response: lastTransientResp, err: err}
		}
		if lastTransientResp != nil {
			_ = lastTransientResp.Body.Close()
		}
		return lastResp, err
	})

	if err != nil {
		var responseErr *preservedResponseError
		if options.PreserveErrorResponses && errors.As(err, &responseErr) {
			res = responseErr.response
			err = nil
		}
	}

	if err != nil {
		classified := classifyCallError(err)
		log.Error("erp request failed",
			slog.String("endpoint", endpoint),
			slog.String("error_type", fmt.Sprintf("%T", err)),
			slog.String("error_kind", string(faultKind(classified))),
		)
		return nil, classified
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
