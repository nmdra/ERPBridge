package mcp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestReconciliationRecoversAfterFullFailedIntervalWithoutRestart(t *testing.T) {
	connectorSpy := &recordingERPConnector{}
	s := NewServer(connectorSpy, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	s.reconciliationInterval = 20 * time.Millisecond

	tool := &Tool{
		Metadata: Metadata{Name: "recovery-test", Version: "1.0.0", IsActive: true, IsServing: true},
		Spec: ToolSpec{
			Description: Description{Short: "recovery"},
			Execution:   Execution{Type: "http", Method: "GET", Endpoint: "http://approved.example/recovery"},
		},
	}
	require.NoError(t, s.store.Save(tool))

	var dependencyFailed atomic.Bool
	dependencyFailed.Store(true)
	s.ReconciliationHooks = &ReconciliationHooks{BeforeAttempt: func(context.Context) error {
		if dependencyFailed.Load() {
			return errors.New("injected dependency failure")
		}
		return nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.StartController(ctx)

	require.Eventually(t, func() bool {
		status := s.ReconciliationStatus()
		return status.AttemptCount >= 2 && !status.Converged && status.LastError != ""
	}, time.Second, 5*time.Millisecond, "one complete retry interval must fail before recovery")
	failedStatus := s.ReconciliationStatus()
	dependencyFailed.Store(false)

	require.Eventually(t, func() bool {
		status := s.ReconciliationStatus()
		if !status.Converged || status.AttemptCount <= failedStatus.AttemptCount || status.LastSuccess.IsZero() {
			return false
		}
		_, err := s.registry.Resolve(tool.Metadata.Name, "")
		return err == nil
	}, time.Second, 5*time.Millisecond)

	request := mcp.CallToolRequest{}
	request.Params.Name = tool.Metadata.Name
	request.Params.Arguments = map[string]any{}
	result, err := s.handleBoundMCPToolCall(tool.Metadata.Name)(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, true, textResult(t, result)["ok"])
	require.Equal(t, 1, connectorSpy.count())
	status := s.ReconciliationStatus()
	require.Empty(t, status.LastError)
	require.NotEmpty(t, status.DesiredStateHash)
	require.Greater(t, status.ObservedGeneration, uint64(0))
}

func TestReconciliationDoesNotClaimConvergenceWhenToolRegistrationFails(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	rejected := &Tool{Metadata: Metadata{Name: "rejected-reconcile", Version: "not-semver", IsActive: true}}
	require.NoError(t, s.store.Save(rejected))

	err := s.runReconciliationAttempt(context.Background())
	require.Error(t, err)
	status := s.ReconciliationStatus()
	require.False(t, status.Converged)
	require.NotEmpty(t, status.LastError)
}

func TestReconciliationAppliesPersistedServingPointerChanges(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	v1 := &Tool{Metadata: Metadata{Name: "pointer-reconcile", Version: "1.0.0", IsActive: true, IsServing: true}}
	v2 := &Tool{Metadata: Metadata{Name: "pointer-reconcile", Version: "2.0.0", IsActive: true}}
	require.NoError(t, s.store.Admit(v1, "test"))
	require.NoError(t, s.store.Admit(v2, "test"))
	s.Reconcile(context.Background())
	serving, err := s.registry.Resolve(v1.Metadata.Name, "")
	require.NoError(t, err)
	require.Equal(t, v1.Metadata.Version, serving.Metadata.Version)

	v2.Metadata.IsServing = true
	require.NoError(t, s.store.Admit(v2, "test"))
	s.Reconcile(context.Background())
	serving, err = s.registry.Resolve(v1.Metadata.Name, "")
	require.NoError(t, err)
	require.Equal(t, v2.Metadata.Version, serving.Metadata.Version)
}

func TestReconciliationRemovesUnqualifiedAliasWhenPersistedPointerIsEmpty(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	tool := &Tool{Metadata: Metadata{Name: "pointer-empty", Version: "1.0.0", IsActive: true, IsServing: true}}
	require.NoError(t, s.store.Admit(tool, "test"))
	s.Reconcile(context.Background())
	_, err := s.registry.Resolve(tool.Metadata.Name, "")
	require.NoError(t, err)

	stored, err := s.store.Get(tool.Metadata.Name, tool.Metadata.Version)
	require.NoError(t, err)
	stored.Metadata.IsServing = false
	require.NoError(t, s.store.Save(stored))
	s.Reconcile(context.Background())
	_, err = s.registry.Resolve(tool.Metadata.Name, "")
	require.Error(t, err)
	require.Nil(t, s.MCPServer().GetTool(tool.Metadata.Name))
}
