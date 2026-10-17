package logger

import (
	"strings"
	"testing"
)

func TestIsSensitiveArgKey(t *testing.T) {
	keys := []string{
		"password", "token", "api_key", "secret", "key", "ssn", "authorization",
		"credential", "private_key", "access_token", "refresh_token", "client_secret",
		"passwd", "apikey", "authkey", "authtoken", "credentialvalue", "clientsecret", "privatekey", "bearer",
	}
	for _, key := range keys {
		for _, variant := range []string{key, " \t" + strings.ToUpper(key) + "\n", "upstream_" + key} {
			t.Run(variant, func(t *testing.T) {
				if !IsSensitiveArgKey(variant) {
					t.Errorf("IsSensitiveArgKey(%q) = false, want true", variant)
				}
			})
		}
	}
}

func TestIsSensitiveArgKeyAllowsReferencesAndSchemaLabels(t *testing.T) {
	for _, key := range []string{
		"", "credentialRef", "credentialSource", "authHeader", "credential_ref", "credential_source", "auth_header",
		"properties", "required", "type", "description", "name", "inputSchema", "outputSchema",
		"passwordHint", "tokenCount", "secretary", "keyboard", "monkey", "client-secret",
	} {
		for _, variant := range []string{key, " \t" + strings.ToUpper(key) + "\n"} {
			t.Run(variant, func(t *testing.T) {
				if IsSensitiveArgKey(variant) {
					t.Errorf("IsSensitiveArgKey(%q) = true, want false", variant)
				}
			})
		}
	}
}

func TestRedactArgsUsesSensitiveArgKeys(t *testing.T) {
	keys := []string{"client_secret", "access_token", "private_key", "passwd", "apikey", "authkey", "authtoken", "credentialvalue", "clientsecret", "privatekey", "bearer"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			input := map[string]any{key: "sensitive-value", "credentialRef": "upstream", "credentialSource": "file", "authHeader": "X-API-Key"}
			result := RedactArgs(input).(map[string]any)
			if result[key] != redactionMarker {
				t.Errorf("RedactArgs()[%q] = %v, want redaction marker", key, result[key])
			}
			for _, reference := range []string{"credentialRef", "credentialSource", "authHeader"} {
				if result[reference] != input[reference] {
					t.Errorf("RedactArgs changed logical field %q", reference)
				}
			}
			if input[key] != "sensitive-value" {
				t.Error("RedactArgs modified its input")
			}
		})
	}
}
