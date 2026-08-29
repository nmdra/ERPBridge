package faults

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestErrorPreservesSafeMetadataWithoutExposingCause(t *testing.T) {
	cause := errors.New("secret upstream response")
	err := New(KindRateLimited, "the service is temporarily rate limited", true, 10*time.Second, cause)

	require.Equal(t, "the service is temporarily rate limited", err.Error())
	require.ErrorIs(t, err, cause)
	require.Equal(t, KindRateLimited, err.Kind)
	require.True(t, err.Retryable)
	require.Equal(t, 10*time.Second, err.RetryAfter)
	require.NotContains(t, err.Error(), "secret")
}

func TestProtocolErrorIsMarkedSeparately(t *testing.T) {
	err := NewProtocol(KindInternal, "internal server error", errors.New("private details"))

	require.True(t, IsProtocol(err))
	require.Equal(t, "internal server error", err.Error())
	require.NotContains(t, err.Error(), "private")
}
