// Package credentials validates logical credential references and resolves them from safe sources.
package credentials

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/nmdra/ERPBridge/internal/metrics"
)

var referencePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CredentialSource identifies where a credential reference is resolved.
type CredentialSource string

const (
	// CredentialsDirEnv configures the optional directory for file-backed credentials.
	CredentialsDirEnv = "ERPBRIDGE_CREDENTIALS_DIR" // #nosec G101 -- environment variable name, not a credential.
	// CredentialSourceEnv resolves the reference from the process environment.
	CredentialSourceEnv CredentialSource = "env"
	// CredentialSourceFile resolves the reference from CredentialsDirEnv/ref.
	CredentialSourceFile CredentialSource = "file"

	maxCredentialFileBytes = 64 << 10
)

// ValidateCredentialSource verifies a credential source. An empty source is
// the wire representation of the backwards-compatible environment default.
func ValidateCredentialSource(source CredentialSource) error {
	if source == "" || source == CredentialSourceEnv || source == CredentialSourceFile {
		return nil
	}
	return errors.New("invalid credential source")
}

// IsFileBacked reports whether source requires a mounted credential file.
func IsFileBacked(source CredentialSource) bool {
	return source == CredentialSourceFile
}

// ValidateReference verifies that ref is a valid environment variable name.
func ValidateReference(ref string) error {
	if ref == "" || !referencePattern.MatchString(ref) {
		return fmt.Errorf("credential reference %q is not a valid environment variable name", ref)
	}
	return nil
}

func recordResolution(source CredentialSource, outcome string) {
	if source != CredentialSourceEnv && source != CredentialSourceFile {
		source = "invalid"
	}
	metrics.CredentialResolutionsTotal.WithLabelValues(string(source), outcome).Inc()
}

func resolutionError(source CredentialSource, outcome, message string) (string, error) {
	recordResolution(source, outcome)
	return "", errors.New(message)
}

// Resolve returns the non-empty value referenced by ref. The optional source
// argument preserves the legacy one-argument environment lookup. File-backed
// resolution reads the mounted file for every call and never falls back to the
// environment or a previous value.
func Resolve(ref string, sources ...CredentialSource) (string, error) {
	source := CredentialSourceEnv
	if len(sources) > 1 {
		return resolutionError(source, "invalid_source", "invalid credential source")
	}
	if len(sources) == 1 && sources[0] != "" {
		source = sources[0]
	}
	if err := ValidateCredentialSource(source); err != nil {
		return resolutionError(source, "invalid_source", err.Error())
	}
	if ref == "" {
		if IsFileBacked(source) {
			return resolutionError(source, "missing_reference", "file credential reference is required")
		}
		return "", nil
	}
	if err := ValidateReference(ref); err != nil {
		return resolutionError(source, "invalid_reference", err.Error())
	}
	if !IsFileBacked(source) {
		value, ok := os.LookupEnv(ref)
		if !ok || value == "" {
			return resolutionError(source, "missing", fmt.Sprintf("credential reference %q is not configured", ref))
		}
		recordResolution(source, "success")
		return value, nil
	}

	dir, ok := os.LookupEnv(CredentialsDirEnv)
	if !ok || dir == "" {
		return resolutionError(source, "directory_missing", "file credential directory is not configured")
	}
	// ref is validated above and cannot contain a path separator or traversal
	// component. Keep the operator-selected directory intact so projected CSI
	// symlinks remain readable.
	path := dir + string(os.PathSeparator) + ref
	// #nosec G304 -- ref is validated and dir is explicit operator configuration.
	file, err := os.Open(path)
	if err != nil {
		return resolutionError(source, "missing", "credential file is not available")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return resolutionError(source, "not_regular", "credential file is not a regular file")
	}
	value, err := io.ReadAll(io.LimitReader(file, maxCredentialFileBytes+1))
	if err != nil {
		return resolutionError(source, "read_error", "credential file could not be read")
	}
	if len(value) == 0 {
		return resolutionError(source, "empty", "credential file is empty")
	}
	if len(value) > maxCredentialFileBytes {
		return resolutionError(source, "oversized", "credential file exceeds maximum size")
	}
	if !utf8.Valid(value) {
		return resolutionError(source, "invalid_content", "credential file contains invalid content")
	}
	for _, r := range string(value) {
		if unicode.IsControl(r) {
			return resolutionError(source, "invalid_content", "credential file contains invalid content")
		}
	}
	recordResolution(source, "success")
	return string(value), nil
}
