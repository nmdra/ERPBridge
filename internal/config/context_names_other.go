//go:build !windows

package config

// Non-Windows context names and registry paths keep their existing semantics.
func validateRawContextNames(_ []byte) error { return nil }

func validatePlatformContexts(_ map[string]Context) error { return nil }

func validatePlatformRegistryContextName(_ string) error { return nil }
