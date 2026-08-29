// Package faults defines the small, transport-independent taxonomy used to
// classify tool and dependency failures without exposing implementation detail.
package faults

import (
	"errors"
	"time"
)

// Kind identifies a safe, machine-readable failure category.
type Kind string

// Failure kinds used by MCP tool and dependency execution.
const (
	KindInvalidInput          Kind = "INVALID_INPUT"
	KindNotFound              Kind = "NOT_FOUND"
	KindPermissionDenied      Kind = "PERMISSION_DENIED"
	KindRateLimited           Kind = "RATE_LIMITED"
	KindConcurrencyLimited    Kind = "CONCURRENCY_LIMITED"
	KindDependencyTimeout     Kind = "DEPENDENCY_TIMEOUT"
	KindDependencyUnavailable Kind = "DEPENDENCY_UNAVAILABLE"
	KindConflict              Kind = "CONFLICT"
	KindInternal              Kind = "INTERNAL"
)

// Error carries safe client-facing metadata and an optional private cause.
// Error intentionally returns Message only; callers may still inspect the
// private cause via errors.Is/errors.As for internal handling and diagnostics.
type Error struct {
	Kind       Kind
	Message    string
	Retryable  bool
	RetryAfter time.Duration
	Protocol   bool
	cause      error
}

func (e *Error) Error() string {
	if e == nil || e.Message == "" {
		return "internal server error"
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// New creates a safe execution or dependency error.
func New(kind Kind, message string, retryable bool, retryAfter time.Duration, cause error) *Error {
	return &Error{
		Kind:       kind,
		Message:    message,
		Retryable:  retryable,
		RetryAfter: retryAfter,
		cause:      cause,
	}
}

// NewProtocol creates a safe infrastructure error that should be returned as a
// JSON-RPC protocol error rather than a CallToolResult error.
func NewProtocol(kind Kind, message string, cause error) *Error {
	err := New(kind, message, false, 0, cause)
	err.Protocol = true
	return err
}

// IsProtocol reports whether err represents a JSON-RPC protocol failure.
func IsProtocol(err error) bool {
	var fault *Error
	return errors.As(err, &fault) && fault.Protocol
}

// As returns the first taxonomy error in err, if present.
func As(err error) (*Error, bool) {
	var fault *Error
	if !errors.As(err, &fault) || fault == nil {
		return nil, false
	}
	return fault, true
}
