// Package credentials resolves environment-backed credential references.
package credentials

import (
	"fmt"
	"os"
	"regexp"
)

var referencePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateReference verifies that ref is a valid environment variable name.
func ValidateReference(ref string) error {
	if ref == "" || !referencePattern.MatchString(ref) {
		return fmt.Errorf("credential reference %q is not a valid environment variable name", ref)
	}
	return nil
}

// Resolve returns the non-empty value referenced by ref.
func Resolve(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if err := ValidateReference(ref); err != nil {
		return "", err
	}
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", fmt.Errorf("credential reference %q is not configured", ref)
	}
	return value, nil
}
