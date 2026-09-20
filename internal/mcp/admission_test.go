package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/nmdra/ERPBridge/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestCanonicalToolDigestExcludesLifecycleAndAdmissionBookkeeping(t *testing.T) {
	left := &Tool{
		APIVersion: "erpbridge.io/v1",
		Kind:       "MCPTool",
		Metadata: Metadata{
			Name:           "employee.lookup",
			Version:        "1.0.0",
			Module:         "hr",
			Status:         "ready",
			IsActive:       true,
			ResourceDigest: "ignored",
			Admission:      &AdmissionRecord{Actor: "first", ApprovedAt: "2026-01-01T00:00:00Z", Decision: admissionDecisionApproved},
		},
		Spec: ToolSpec{InputSchema: InputSchema{Type: "object", Properties: map[string]Property{
			"employee": {Type: "string", Description: "Employee identifier"},
		}}},
	}
	right := *left
	right.Metadata = left.Metadata
	right.Metadata.Status = "degraded"
	right.Metadata.IsActive = false
	right.Metadata.ResourceDigest = "different"
	right.Metadata.Admission = &AdmissionRecord{Actor: "second", ApprovedAt: "2026-02-01T00:00:00Z", Decision: admissionDecisionApproved}

	leftDigest, err := CanonicalToolDigest(left)
	require.NoError(t, err)
	rightDigest, err := CanonicalToolDigest(&right)
	require.NoError(t, err)
	require.Equal(t, leftDigest, rightDigest)

	right.Spec.Description.Short = "changed executable description"
	changedDigest, err := CanonicalToolDigest(&right)
	require.NoError(t, err)
	require.NotEqual(t, leftDigest, changedDigest)
}

func TestToolApplyPersistsExplicitServingSelection(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	apply := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		s.handleToolAPI(response, req)
		return response
	}
	v1 := `{"metadata":{"name":"serving-test","version":"1.0.0"},"spec":{"description":{"short":"v1"}}}`
	v2 := `{"metadata":{"name":"serving-test","version":"2.0.0"},"spec":{"description":{"short":"v2"}}}`
	require.Equal(t, http.StatusCreated, apply(v1).Code)
	require.Equal(t, http.StatusCreated, apply(v2).Code)

	storedV1, err := s.store.Get("serving-test", "1.0.0")
	require.NoError(t, err)
	storedV2, err := s.store.Get("serving-test", "2.0.0")
	require.NoError(t, err)
	require.True(t, storedV1.Metadata.IsServing)
	require.False(t, storedV2.Metadata.IsServing)

	v2Serving := `{"metadata":{"name":"serving-test","version":"2.0.0","isServing":true},"spec":{"description":{"short":"v2"}}}`
	require.Equal(t, http.StatusCreated, apply(v2Serving).Code)
	storedV1, err = s.store.Get("serving-test", "1.0.0")
	require.NoError(t, err)
	storedV2, err = s.store.Get("serving-test", "2.0.0")
	require.NoError(t, err)
	require.False(t, storedV1.Metadata.IsServing)
	require.True(t, storedV2.Metadata.IsServing)
}

func TestToolApplyRejectsChangedContentForAdmittedNameVersion(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })

	apply := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(body))
		req = req.WithContext(WithCallerIdentity(context.Background(), CallerIdentity{PrincipalID: "reviewer-1", IsAdmin: true}))
		response := httptest.NewRecorder()
		s.handleToolAPI(response, req)
		return response
	}

	first := apply(testToolJSON)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	var firstBody map[string]any
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	digest, ok := firstBody["resourceDigest"].(string)
	require.True(t, ok)
	require.Len(t, digest, 64)

	identical := apply(testToolJSON)
	require.Equal(t, http.StatusCreated, identical.Code, identical.Body.String())

	changed := apply(`{"metadata":{"name":"test-tool","version":"1.0.0"},"spec":{"description":{"short":"changed"}}}`)
	require.Equal(t, http.StatusConflict, changed.Code, changed.Body.String())
	var conflict controlPlaneErrorEnvelope
	require.NoError(t, json.Unmarshal(changed.Body.Bytes(), &conflict))
	require.Equal(t, ErrorRegistryConflict, conflict.Error)

	stored, err := s.store.Get("test-tool", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, "test", stored.Spec.Description.Short)
	require.Equal(t, digest, stored.Metadata.ResourceDigest)
	require.NotNil(t, stored.Metadata.Admission)
	require.Equal(t, "reviewer-1", stored.Metadata.Admission.Actor)
	require.Equal(t, admissionDecisionApproved, stored.Metadata.Admission.Decision)
}

func TestToolApplyRejectsReservedExactRevisionNamespace(t *testing.T) {
	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, ":memory:")
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	req := httptest.NewRequest(http.MethodPost, "/apis/erpbridge.io/v1/tools", bytes.NewBufferString(`{"metadata":{"name":"orders.rev_MS4wLjA","version":"1.0.0"},"spec":{"description":{"short":"collision"}}}`))
	response := httptest.NewRecorder()
	s.handleToolAPI(response, req)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
}

func TestLegacyStoreMigrationPersistsSemanticServingRevisionAndAdmission(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	seed, err := NewStore(dbPath)
	require.NoError(t, err)
	for _, version := range []string{"2.0.0", "10.0.0"} {
		require.NoError(t, seed.Save(&Tool{
			APIVersion: "erpbridge.io/v1",
			Kind:       "MCPTool",
			Metadata:   Metadata{Name: "legacy-serving", Version: version, IsActive: true},
			Spec:       ToolSpec{Description: Description{Short: "legacy"}},
		}))
	}
	require.NoError(t, seed.Close())

	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, dbPath)
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	s.Reconcile(context.Background())
	serving, err := s.registry.Resolve("legacy-serving", "")
	require.NoError(t, err)
	require.Equal(t, "10.0.0", serving.Metadata.Version)

	stored, err := s.store.Get("legacy-serving", "10.0.0")
	require.NoError(t, err)
	require.True(t, stored.Metadata.IsServing)
	require.Len(t, stored.Metadata.ResourceDigest, 64)
	require.NotNil(t, stored.Metadata.Admission)
	require.Equal(t, "legacy-migration", stored.Metadata.Admission.Actor)
}

func TestRestartDoesNotCreateFallbackAfterServingWithdrawal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "withdrawn-serving.db")
	seed, err := NewStore(dbPath)
	require.NoError(t, err)
	v1 := &Tool{Metadata: Metadata{Name: "no-fallback", Version: "1.0.0", IsActive: true, IsServing: true}}
	v2 := &Tool{Metadata: Metadata{Name: "no-fallback", Version: "2.0.0", IsActive: true}}
	require.NoError(t, seed.Admit(v1, "test"))
	require.NoError(t, seed.Admit(v2, "test"))
	require.NoError(t, seed.Delete(v1.Metadata.Name, v1.Metadata.Version))
	require.NoError(t, seed.Close())

	s := NewServer(nil, nil, logger.Init(), RateLimitConfig{RequestsPerSecond: 100, Burst: 100}, dbPath)
	t.Cleanup(func() { require.NoError(t, s.store.Close()) })
	s.Reconcile(context.Background())
	_, err = s.registry.Resolve(v1.Metadata.Name, "")
	require.Error(t, err)
	exact, err := s.registry.Resolve(v2.Metadata.Name, v2.Metadata.Version)
	require.NoError(t, err)
	require.Equal(t, v2.Metadata.Version, exact.Metadata.Version)
}
