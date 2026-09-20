package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	canonicalToolV1           = "erpbridge-tool-json-v1"
	admissionDecisionApproved = "approved"
)

// ErrAdmissionConflict means an admitted name and version was presented with
// different executable content. Admitted revisions are immutable.
var ErrAdmissionConflict = errors.New("admitted tool revision conflicts with stored content")

// AdmissionRecord binds an operator decision to canonical executable content.
// It is bookkeeping and is not itself part of the executable-content digest.
type AdmissionRecord struct {
	Actor            string `json:"actor"`
	ApprovedAt       string `json:"approvedAt"`
	Decision         string `json:"decision"`
	Canonicalization string `json:"canonicalization"`
}

func cloneTool(tool *Tool) (*Tool, error) {
	if tool == nil {
		return nil, fmt.Errorf("tool is required")
	}
	handler := tool.Handler
	data, err := json.Marshal(tool)
	if err != nil {
		return nil, fmt.Errorf("marshal tool clone: %w", err)
	}
	var cloned Tool
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("unmarshal tool clone: %w", err)
	}
	cloned.Handler = handler
	return &cloned, nil
}

// CanonicalToolDigest returns the SHA-256 digest of executable resource
// content. Runtime lifecycle state, the digest field, and admission bookkeeping
// are excluded. encoding/json provides deterministic map-key ordering.
func CanonicalToolDigest(tool *Tool) (string, error) {
	if tool == nil {
		return "", fmt.Errorf("tool is required")
	}

	canonical, err := cloneTool(tool)
	if err != nil {
		return "", fmt.Errorf("clone tool for canonicalization: %w", err)
	}
	canonical.Handler = nil
	canonical.Metadata.Status = ""
	canonical.Metadata.IsActive = false
	canonical.Metadata.IsServing = false
	canonical.Metadata.ResourceDigest = ""
	canonical.Metadata.Admission = nil

	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal canonical tool: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func newAdmissionRecord(actor string, now time.Time) *AdmissionRecord {
	if actor == "" {
		actor = "control-plane-operator"
	}
	return &AdmissionRecord{
		Actor:            actor,
		ApprovedAt:       now.UTC().Format(time.RFC3339Nano),
		Decision:         admissionDecisionApproved,
		Canonicalization: canonicalToolV1,
	}
}
