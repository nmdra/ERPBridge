// Package config provides configuration parsing, context definitions, and environment variable overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/goccy/go-yaml"
)

// DefaultContextName is the default context identifier.
const DefaultContextName = "local"

// ErrContextNotFound indicates that the selected context is not configured.
var ErrContextNotFound = errors.New("context not found")

var contextNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateContextName validates a context name before it is used for selection
// or as part of a local registry path.
func ValidateContextName(name string) error {
	if !contextNamePattern.MatchString(name) {
		return fmt.Errorf("invalid context name %q: use 1-64 letters, numbers, dots, underscores, or hyphens", name)
	}
	return nil
}

// AuthConfig defines the authentication parameters for communicating with an ERP or bridge server.
type AuthConfig struct {
	Type     string `yaml:"type"` // api-key | basic | bearer
	Header   string `yaml:"header"`
	Key      string `yaml:"key"`
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
}

// Context defines connection and authentication settings for a specific target environment.
type Context struct {
	Server    string     `yaml:"server"`
	MCPServer string     `yaml:"mcp-server"`
	ERPBase   string     `yaml:"erp-base"`
	APIToken  string     `yaml:"api-token"`
	Auth      AuthConfig `yaml:"auth"`
}

// Config represents the complete CLI configuration containing multiple contexts.
type Config struct {
	CurrentContext string             `yaml:"current-context"`
	Contexts       map[string]Context `yaml:"contexts"`
}

// Load reads the config file then applies environment variable overrides.
func Load() (*Config, error) {
	cfg, err := loadFile()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg = defaultConfig()
	}
	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]Context)
	}
	applyEnvOverrides(cfg)
	if _, err := cfg.EffectiveContext(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadFile() (*Config, error) {
	path := filepath.Clean(configPath())
	// #nosec G304 -- config path is resolved within user's home directory
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Expand ${VAR} references inside the YAML before parsing
	expanded := os.ExpandEnv(string(data))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BRIDGE_CONTEXT"); v != "" {
		cfg.CurrentContext = v
	}

	ctx := cfg.ActiveContext()

	if v := os.Getenv("BRIDGE_SERVER"); v != "" {
		ctx.Server = v
	}
	if v := os.Getenv("BRIDGE_MCP_SERVER"); v != "" {
		ctx.MCPServer = v
	}
	if v := os.Getenv("BRIDGE_ERP_BASE"); v != "" {
		ctx.ERPBase = v
	}
	if v := os.Getenv("BRIDGE_API_TOKEN"); v != "" {
		ctx.APIToken = v
	}
	if v := os.Getenv("BRIDGE_AUTH_TYPE"); v != "" {
		ctx.Auth.Type = v
	}
	if v := os.Getenv("BRIDGE_API_KEY"); v != "" {
		ctx.Auth.Key = v
	}
	if v := os.Getenv("BRIDGE_AUTH_HEADER"); v != "" {
		ctx.Auth.Header = v
	}
	if v := os.Getenv("BRIDGE_TOKEN"); v != "" {
		ctx.Auth.Token = v
	}
	if v := os.Getenv("BRIDGE_USERNAME"); v != "" {
		ctx.Auth.Username = v
	}
	if v := os.Getenv("BRIDGE_PASSWORD"); v != "" {
		ctx.Auth.Key = v // Basic Auth password maps to key field
	}

	if _, ok := cfg.Contexts[cfg.CurrentContext]; ok {
		cfg.Contexts[cfg.CurrentContext] = ctx
	}
}

// ResolveContext returns a named context, or the current context when name is
// empty. It is the single error-producing context lookup used by consumers.
func (c *Config) ResolveContext(name string) (Context, error) {
	if c == nil {
		return Context{}, fmt.Errorf("%w: configuration is nil", ErrContextNotFound)
	}
	if name == "" {
		name = c.CurrentContext
	}
	if err := ValidateContextName(name); err != nil {
		return Context{}, fmt.Errorf("%w: %w", ErrContextNotFound, err)
	}
	if c.Contexts == nil {
		return Context{}, fmt.Errorf("%w: %q is not configured", ErrContextNotFound, name)
	}
	ctx, ok := c.Contexts[name]
	if !ok {
		return Context{}, fmt.Errorf("%w: %q is not configured", ErrContextNotFound, name)
	}
	return ctx, nil
}

// EffectiveContext returns the configured current context.
func (c *Config) EffectiveContext() (Context, error) {
	return c.ResolveContext("")
}

// ActiveContext returns the currently active context from the configuration.
//
// Deprecated: use EffectiveContext when a missing context must be reported.
func (c *Config) ActiveContext() Context {
	ctx, err := c.EffectiveContext()
	if err != nil {
		return Context{}
	}
	return ctx
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bridgectl", "config.yaml")
}

func defaultConfig() *Config {
	return &Config{
		CurrentContext: DefaultContextName,
		Contexts:       map[string]Context{DefaultContextName: defaultContext()},
	}
}

func defaultContext() Context {
	return Context{
		Server:    "http://localhost:8082",
		MCPServer: "http://localhost:8080",
		ERPBase:   "http://localhost:8081",
		Auth:      AuthConfig{Type: "api-key", Header: "X-API-Key"},
	}
}
