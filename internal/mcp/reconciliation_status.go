package mcp

import (
	"context"
	"time"
)

const defaultReconcileInterval = 10 * time.Second

// ReconciliationStatus is a safe process-local observation. Desired resources
// and withdrawal tombstones remain in SQLite; cycle telemetry need not persist.
type ReconciliationStatus struct {
	AttemptCount       uint64    `json:"attemptCount"`
	LastAttempt        time.Time `json:"lastAttempt,omitempty"`
	LastSuccess        time.Time `json:"lastSuccess,omitempty"`
	LastError          string    `json:"lastError,omitempty"`
	DesiredStateHash   string    `json:"desiredStateHash,omitempty"`
	ObservedGeneration uint64    `json:"observedGeneration"`
	Converged          bool      `json:"converged"`
}

// ReconciliationHooks supports deterministic dependency-failure injection in
// conformance tests. Production callers leave it nil.
type ReconciliationHooks struct {
	BeforeAttempt func(context.Context) error
}

// ReconciliationStatus returns a copy of the process-local controller status.
func (s *Server) ReconciliationStatus() ReconciliationStatus {
	s.reconcileStatusMu.RLock()
	defer s.reconcileStatusMu.RUnlock()
	return s.reconcileStatus
}

func (s *Server) runReconciliationAttempt(ctx context.Context) error {
	s.reconcileStatusMu.Lock()
	s.reconcileStatus.AttemptCount++
	s.reconcileStatus.LastAttempt = time.Now().UTC()
	s.reconcileStatusMu.Unlock()

	if s.ReconciliationHooks != nil && s.ReconciliationHooks.BeforeAttempt != nil {
		if err := s.ReconciliationHooks.BeforeAttempt(ctx); err != nil {
			s.recordReconciliationFailure()
			return err
		}
	}

	s.pluginLifecycleMu.Lock()
	err := s.reconcileLocked(ctx)
	generation := s.lifecycleGeneration
	s.pluginLifecycleMu.Unlock()
	if err != nil {
		s.recordReconciliationFailure()
		return err
	}

	s.mu.RLock()
	reconciledHash := s.lastDesiredHash
	s.mu.RUnlock()
	s.reconcileStatusMu.Lock()
	s.reconcileStatus.LastSuccess = time.Now().UTC()
	s.reconcileStatus.LastError = ""
	s.reconcileStatus.DesiredStateHash = reconciledHash
	s.reconcileStatus.ObservedGeneration = generation
	s.reconcileStatus.Converged = true
	s.reconcileStatusMu.Unlock()
	return nil
}

func (s *Server) recordReconciliationFailure() {
	s.reconcileStatusMu.Lock()
	s.reconcileStatus.LastError = "reconciliation attempt failed"
	s.reconcileStatus.Converged = false
	s.reconcileStatusMu.Unlock()
}

func (s *Server) reconciliationIntervalValue() time.Duration {
	if s.reconciliationInterval > 0 {
		return s.reconciliationInterval
	}
	return defaultReconcileInterval
}
