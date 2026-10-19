package config

import (
	"strings"
	"testing"
)

func TestContextIdentityLoadChecksExpandedKeys(t *testing.T) {
	identityConfigHome(t, "current-context: local\ncontexts:\n  local: {}\n  '${IDENTITY_LOWER}': {}\n  '${IDENTITY_UPPER}': {}\n")
	t.Setenv("IDENTITY_LOWER", "prod")
	t.Setenv("IDENTITY_UPPER", "Prod")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "distinct names") {
		t.Fatalf("expanded names must not alias: %v", err)
	}
}

func TestContextIdentityReservedDeviceFamilies(t *testing.T) {
	for _, prefix := range []string{"COM", "LPT"} {
		for digit := '1'; digit <= '9'; digit++ {
			name := prefix + string(digit)
			for _, candidate := range []string{name, strings.ToLower(name), name + ".test"} {
				if err := ValidateRegistryContextName(candidate); err == nil {
					t.Fatalf("reserved name accepted: %q", candidate)
				}
			}
		}
	}
	for _, name := range []string{"COM0", "LPT0", "COM10", "LPT10", "x.CON", "NUL-test", "CON_test", "PRName", "AUXiliary"} {
		if err := ValidateRegistryContextName(name); err != nil {
			t.Fatalf("safe lookalike %q refused: %v", name, err)
		}
	}
}
