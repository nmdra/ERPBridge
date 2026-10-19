package config

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
)

// validateRawContextNames checks keys without expanding environment references.
// Values and unknown document fields are deliberately not decoded as Context.
func validateRawContextNames(data []byte) error {
	document := make(map[string]any)
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse configuration: %w", err)
	}
	contexts, _ := document["contexts"].(map[string]any)
	names := make(map[string]Context, len(contexts))
	for name := range contexts {
		names[name] = Context{}
	}
	return validatePlatformContexts(names)
}

func validatePlatformContexts(contexts map[string]Context) error {
	seen := make(map[string]bool, len(contexts))
	for name := range contexts {
		folded := strings.ToLower(name)
		if seen[folded] {
			// Do not include document values or potentially expanded keys in errors.
			return fmt.Errorf("context names differ only by case on Windows; choose distinct names")
		}
		seen[folded] = true
	}
	return nil
}

func validatePlatformRegistryContextName(name string) error {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(stem)
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL"
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		reserved = true
	}
	if reserved {
		return fmt.Errorf("invalid registry context name %q: reserved Windows device name", name)
	}
	return nil
}
