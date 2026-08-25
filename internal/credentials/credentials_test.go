package credentials

import "testing"

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
