package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Setenv("ERP_TEST_KEY", "secret")
	value, err := Resolve("ERP_TEST_KEY")
	if err != nil || value != "secret" {
		t.Fatalf("Resolve() = %q, %v", value, err)
	}
	if _, err := Resolve("ERP_MISSING_KEY"); err == nil {
		t.Fatal("Resolve() accepted an unset reference")
	}
}

func TestResolveCredentialSources(t *testing.T) {
	t.Setenv("ERP_TEST_KEY", "environment-secret")
	if value, err := Resolve("ERP_TEST_KEY", CredentialSourceEnv); err != nil || value != "environment-secret" {
		t.Fatalf("environment source = %q, %v", value, err)
	}

	dir := t.TempDir()
	t.Setenv(CredentialsDirEnv, dir)
	if err := os.WriteFile(filepath.Join(dir, "ERP_TEST_KEY"), []byte("file-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := Resolve("ERP_TEST_KEY", CredentialSourceFile)
	if err != nil || value != "file-secret" {
		t.Fatalf("file source = %q, %v", value, err)
	}
}

func TestResolveFileFailsClosedWithoutEnvironmentFallback(t *testing.T) {
	t.Setenv("ERP_TEST_KEY", "environment-secret")
	dir := t.TempDir()
	t.Setenv(CredentialsDirEnv, dir)
	_, err := Resolve("ERP_TEST_KEY", CredentialSourceFile)
	if err == nil {
		t.Fatal("file source accepted a missing file")
	}
	if strings.Contains(err.Error(), "environment-secret") || strings.Contains(err.Error(), dir) {
		t.Fatalf("file error leaked secret or path: %v", err)
	}
}

func TestResolveFileAcceptsProjectedSymlinkAndExactContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CredentialsDirEnv, dir)
	version := filepath.Join(dir, "..data-v1")
	if err := os.WriteFile(version, []byte(" secret-with-spaces "), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(version, filepath.Join(dir, "ERP_TEST_KEY")); err != nil {
		t.Fatal(err)
	}
	value, err := Resolve("ERP_TEST_KEY", CredentialSourceFile)
	if err != nil || value != " secret-with-spaces " {
		t.Fatalf("projected file = %q, %v", value, err)
	}
}

func TestResolveFileRejectsInvalidContentAndBounds(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "control", data: []byte("secret\n")},
		{name: "oversized", data: []byte(strings.Repeat("x", maxCredentialFileBytes+1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(CredentialsDirEnv, dir)
			if err := os.WriteFile(filepath.Join(dir, "ERP_TEST_KEY"), tt.data, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Resolve("ERP_TEST_KEY", CredentialSourceFile)
			if err == nil {
				t.Fatal("invalid file content was accepted")
			}
		})
	}
}

func TestCredentialSourceValidation(t *testing.T) {
	if err := ValidateCredentialSource(CredentialSourceEnv); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCredentialSource(CredentialSourceFile); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCredentialSource("vault"); err == nil {
		t.Fatal("invalid credential source was accepted")
	}
	if !IsFileBacked(CredentialSourceFile) || IsFileBacked(CredentialSourceEnv) {
		t.Fatal("unexpected file-backed source classification")
	}
}

func TestValidateReference(t *testing.T) {
	for _, ref := range []string{"ERP_KEY", "_ERP_KEY", "erpKey1"} {
		if err := ValidateReference(ref); err != nil {
			t.Errorf("ValidateReference(%q): %v", ref, err)
		}
	}
	for _, ref := range []string{"", "1ERP_KEY", "ERP-KEY"} {
		if err := ValidateReference(ref); err == nil {
			t.Errorf("ValidateReference(%q) accepted invalid reference", ref)
		}
	}
}
