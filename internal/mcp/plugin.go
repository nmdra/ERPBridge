package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// PluginAPIVersion is the version of the external-plugin resource contract.
	PluginAPIVersion = "erpbridge.io/v1"
	// PluginKind is the Kubernetes-style resource kind for plugin definitions.
	PluginKind = "Plugin"
	// PluginBindingKind is the Kubernetes-style resource kind for bindings.
	PluginBindingKind = "PluginBinding"

	// PluginProtocolVersion is the wire-protocol version sent to plugins.
	PluginProtocolVersion = "v1"

	// PluginPhaseAfterResponse runs after successful tool response validation.
	PluginPhaseAfterResponse = "after_response"

	// PluginFailurePolicyContinue preserves the original result on plugin failure.
	PluginFailurePolicyContinue = "continue"
	// PluginFailurePolicyFail returns a generic failure on plugin failure.
	PluginFailurePolicyFail = "fail"

	// maxPluginJSONBytes bounds each request and response JSON document.
	maxPluginJSONBytes = 1 << 20

	// pluginMaxTimeoutMilliseconds prevents a declarative resource from
	// holding an invocation open without bound.
	pluginMaxTimeoutMilliseconds = 5 * 60 * 1000
	pluginProcessPath            = "/v1/process"
)

// MaxPluginJSONBytes is the maximum size of an external-plugin JSON document.
const MaxPluginJSONBytes = maxPluginJSONBytes

// Plugin describes an already-running external plugin endpoint. ERPBridge
// stores this resource but never starts or deploys the referenced process.
type Plugin struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   PluginMetadata `json:"metadata"`
	Spec       PluginSpec     `json:"spec"`
}

// PluginMetadata contains a plugin's exact identity and lifecycle state.
type PluginMetadata struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	IsActive bool   `json:"isActive,omitempty"`
}

// PluginSpec contains the endpoint and invocation timeout for a plugin.
type PluginSpec struct {
	Endpoint            string `json:"endpoint"`
	TimeoutMilliseconds int    `json:"timeoutMilliseconds"`
}

// PluginRef identifies an exact plugin version.
type PluginRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolRef identifies an exact MCP tool version.
type ToolRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// PluginBinding connects one exact plugin version to one exact tool version.
type PluginBinding struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Metadata   PluginBindingMetadata `json:"metadata"`
	Spec       PluginBindingSpec     `json:"spec"`
}

// PluginBindingMetadata contains a binding's identity and lifecycle state.
type PluginBindingMetadata struct {
	Name     string `json:"name"`
	IsActive bool   `json:"isActive,omitempty"`
}

// PluginBindingSpec defines when and how an exact plugin is applied.
type PluginBindingSpec struct {
	PluginRef     PluginRef      `json:"pluginRef"`
	ToolRef       ToolRef        `json:"toolRef"`
	Phase         string         `json:"phase"`
	Priority      int            `json:"priority"`
	FailurePolicy string         `json:"failurePolicy,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
}

// ToolIdentity is the exact tool identity included in a plugin invocation.
type ToolIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// PluginInvocation is the only request body sent to an external plugin.
// It deliberately has no original arguments, inbound headers, credentials, or
// caller identity fields.
type PluginInvocation struct {
	ProtocolVersion string         `json:"protocolVersion"`
	InvocationID    string         `json:"invocationId"`
	Tool            ToolIdentity   `json:"tool"`
	Result          any            `json:"result"`
	Config          map[string]any `json:"config,omitempty"`
}

// PluginResponse is the generic response envelope accepted from a plugin.
type PluginResponse struct {
	Result any `json:"result"`
}

// Validate checks the exact tool identity carried by a plugin invocation.
func (i PluginInvocation) Validate() error {
	if strings.TrimSpace(i.Tool.Name) == "" {
		return fmt.Errorf("tool.name is required")
	}
	if strings.TrimSpace(i.Tool.Version) == "" {
		return fmt.Errorf("tool.version is required")
	}
	if _, err := semver.NewVersion(i.Tool.Version); err != nil {
		return fmt.Errorf("tool.version must be a valid semver version: %w", err)
	}
	return nil
}

// Validate checks a plugin resource without contacting its endpoint.
func (p *Plugin) Validate() error {
	if p == nil {
		return fmt.Errorf("plugin is required")
	}
	if p.APIVersion != PluginAPIVersion {
		return fmt.Errorf("apiVersion must be %q", PluginAPIVersion)
	}
	if p.Kind != PluginKind {
		return fmt.Errorf("kind must be %q", PluginKind)
	}
	if strings.TrimSpace(p.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(p.Metadata.Version) == "" {
		return fmt.Errorf("metadata.version is required")
	}
	if _, err := semver.NewVersion(p.Metadata.Version); err != nil {
		return fmt.Errorf("metadata.version must be a valid semver version: %w", err)
	}
	if p.Spec.TimeoutMilliseconds <= 0 || p.Spec.TimeoutMilliseconds > pluginMaxTimeoutMilliseconds {
		return fmt.Errorf("spec.timeoutMilliseconds must be between 1 and %d", pluginMaxTimeoutMilliseconds)
	}

	u, err := url.Parse(strings.TrimSpace(p.Spec.Endpoint))
	if err != nil {
		return fmt.Errorf("spec.endpoint is invalid: %w", err)
	}
	if err := validatePluginEndpoint(u); err != nil {
		return fmt.Errorf("spec.endpoint: %w", err)
	}
	return nil
}

// Validate checks a binding resource without resolving its references.
func (b *PluginBinding) Validate() error {
	if b == nil {
		return fmt.Errorf("plugin binding is required")
	}
	if b.APIVersion != PluginAPIVersion {
		return fmt.Errorf("apiVersion must be %q", PluginAPIVersion)
	}
	if b.Kind != PluginBindingKind {
		return fmt.Errorf("kind must be %q", PluginBindingKind)
	}
	if strings.TrimSpace(b.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(b.Spec.PluginRef.Name) == "" {
		return fmt.Errorf("spec.pluginRef.name is required")
	}
	if strings.TrimSpace(b.Spec.PluginRef.Version) == "" {
		return fmt.Errorf("spec.pluginRef.version is required")
	}
	if _, err := semver.NewVersion(b.Spec.PluginRef.Version); err != nil {
		return fmt.Errorf("spec.pluginRef.version must be a valid semver version: %w", err)
	}
	if strings.TrimSpace(b.Spec.ToolRef.Name) == "" {
		return fmt.Errorf("spec.toolRef.name is required")
	}
	if strings.TrimSpace(b.Spec.ToolRef.Version) == "" {
		return fmt.Errorf("spec.toolRef.version is required")
	}
	if _, err := semver.NewVersion(b.Spec.ToolRef.Version); err != nil {
		return fmt.Errorf("spec.toolRef.version must be a valid semver version: %w", err)
	}
	if b.Spec.Phase != PluginPhaseAfterResponse {
		return fmt.Errorf("spec.phase must be %q", PluginPhaseAfterResponse)
	}
	if b.Spec.Priority < 0 {
		return fmt.Errorf("spec.priority must not be negative")
	}
	if b.Spec.FailurePolicy != "" && b.Spec.FailurePolicy != PluginFailurePolicyContinue && b.Spec.FailurePolicy != PluginFailurePolicyFail {
		return fmt.Errorf("spec.failurePolicy must be %q or %q", PluginFailurePolicyContinue, PluginFailurePolicyFail)
	}
	if _, err := json.Marshal(b.Spec.Config); err != nil {
		return fmt.Errorf("spec.config must be valid JSON: %w", err)
	}
	return nil
}

// EffectiveFailurePolicy returns the safe default when a manifest omits the
// optional failure policy.
func (b *PluginBinding) EffectiveFailurePolicy() string {
	if b == nil || b.Spec.FailurePolicy == "" {
		return PluginFailurePolicyContinue
	}
	return b.Spec.FailurePolicy
}

// ToolKey returns the exact tool identity key used by the active registry.
func (b *PluginBinding) ToolKey() string {
	if b == nil {
		return ""
	}
	return b.Spec.ToolRef.Name + "@" + b.Spec.ToolRef.Version
}

func validatePluginEndpoint(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return fmt.Errorf("must be an absolute http or https URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return fmt.Errorf("must use http or https")
	}
	if endpoint.User != nil {
		return fmt.Errorf("userinfo is not allowed")
	}
	if endpoint.RawQuery != "" {
		return fmt.Errorf("query parameters are not allowed")
	}
	if endpoint.Fragment != "" {
		return fmt.Errorf("fragments are not allowed")
	}
	return nil
}

func (p *Plugin) processURL() (*url.URL, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimSpace(p.Spec.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid plugin endpoint")
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, pluginProcessPath) {
		path += pluginProcessPath
	}
	if path == pluginProcessPath && u.Path == "" {
		path = pluginProcessPath
	}
	u.Path = path
	u.RawPath = ""
	return u, nil
}
