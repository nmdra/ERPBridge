package logger

import (
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/m-mizutani/masq"
	"github.com/nmdra/ERPBridge/internal/types"
)

const (
	redactedPasswordKey = "password"
	redactedAPIKey      = "api_key"
	redactionMarker     = "[REDACTED]"
)

var redactAttr = masq.New(
	masq.WithType[types.APIToken](),
	masq.WithType[types.Password](),
	masq.WithType[types.AuthHeader](),
	masq.WithType[types.SecretKey](),
	masq.WithType[types.PII](),
	masq.WithTag("secret"),
	masq.WithTag("pii"),
	masq.WithTag("masq"),
	masq.WithFieldPrefix("Secret"),
	masq.WithFieldPrefix("Private"),
	masq.WithFieldName(redactedPasswordKey),
	masq.WithFieldName("token"),
	masq.WithFieldName(redactedAPIKey),
	masq.WithFieldName("secret"),
	masq.WithFieldName("key"),
	masq.WithFieldName("authorization"),
	masq.WithFieldName("ssn"),
	masq.WithFieldName("national_id"),
	masq.WithFieldName("bank_account"),
	masq.WithRegex(regexp.MustCompile(`(?i)bearer\s+\S+`)),
)

// RedactAttr is the shared slog attribute redactor used by all log sinks.
func RedactAttr(groups []string, attr slog.Attr) slog.Attr {
	return redactAttr(groups, attr)
}

// RedactArgs returns a recursively redacted copy of JSON-like arguments.
func RedactArgs(value any) any {
	return redactValue(reflect.ValueOf(value))
}

// RedactHeaders returns a copy of HTTP headers with credential-bearing values
// replaced before they are attached to logs or diagnostics.
func RedactHeaders(headers http.Header) http.Header {
	redacted := make(http.Header, len(headers))
	for key, values := range headers {
		if sensitiveHeaderKey(key) {
			redacted[http.CanonicalHeaderKey(key)] = []string{redactionMarker}
			continue
		}
		redacted[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
	return redacted
}

func redactValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}

	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return redactValue(value.Elem())
	}

	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return value.Interface()
		}
		result := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if sensitiveArgKey(key) {
				result[key] = redactionMarker
				continue
			}
			result[key] = redactValue(iter.Value())
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = redactValue(value.Index(i))
		}
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return redactValue(value.Elem())
	default:
		return value.Interface()
	}
}

func sensitiveArgKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, sensitive := range []string{
		redactedPasswordKey, "token", redactedAPIKey, "secret", "key", "ssn", "authorization",
		"credential", "private_key", "access_token", "refresh_token", "client_secret",
	} {
		if key == sensitive || strings.HasSuffix(key, "_"+sensitive) {
			return true
		}
	}
	return false
}

func sensitiveHeaderKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key":
		return true
	default:
		return false
	}
}
