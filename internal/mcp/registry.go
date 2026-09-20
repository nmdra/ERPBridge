package mcp

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ToolRegistry manages multiple versions of tools and resolves them for the LLM.
type ToolRegistry struct {
	tools   map[string]map[string]*Tool // name -> version -> immutable Tool
	serving map[string]string           // name -> operator-selected version
}

// NewToolRegistry creates a new instance of ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:   make(map[string]map[string]*Tool),
		serving: make(map[string]string),
	}
}

// Add adds a tool and preserves the historical first-registration serving
// default for programmatic callers.
func (r *ToolRegistry) Add(t *Tool) error {
	return r.add(t, true)
}

// AddExplicit adds persisted state without inventing a serving pointer.
func (r *ToolRegistry) AddExplicit(t *Tool) error {
	return r.add(t, false)
}

func (r *ToolRegistry) add(t *Tool, allowServingDefault bool) error {
	name := t.Metadata.Name
	version := t.Metadata.Version

	if _, err := semver.NewVersion(version); err != nil {
		return fmt.Errorf("invalid semver version %s for tool %s: %w", version, name, err)
	}

	if _, ok := r.tools[name]; !ok {
		r.tools[name] = make(map[string]*Tool)
	}

	cloned, err := cloneTool(t)
	if err != nil {
		return err
	}
	r.tools[name][version] = cloned
	if t.Metadata.IsServing || (allowServingDefault && r.serving[name] == "") {
		if err := r.SetServing(name, version); err != nil {
			return err
		}
	} else if !allowServingDefault && r.serving[name] == version {
		delete(r.serving, name)
	}
	return nil
}

// Resolve finds the appropriate version of a tool based on an optional version constraint.
// If no version is specified, it returns the latest stable version.
// It only returns tools that are active.
func (r *ToolRegistry) Resolve(name string, versionConstraint string) (*Tool, error) {
	versions, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	if versionConstraint != "" {
		// If explicit version like "list_employees@1.0.0" is passed
		if t, ok := versions[versionConstraint]; ok {
			if !t.Metadata.IsActive {
				return nil, fmt.Errorf("tool %s@%s is inactive", name, versionConstraint)
			}
			return cloneTool(t)
		}

		// If it's a semver constraint like "^1.0.0"
		c, err := semver.NewConstraint(versionConstraint)
		if err != nil {
			return nil, fmt.Errorf("invalid version constraint %s: %w", versionConstraint, err)
		}

		var bestVersion *semver.Version
		var bestTool *Tool

		for vStr, t := range versions {
			if !t.Metadata.IsActive {
				continue
			}
			v, _ := semver.NewVersion(vStr)
			if c.Check(v) {
				if bestVersion == nil || v.GreaterThan(bestVersion) {
					bestVersion = v
					bestTool = t
				}
			}
		}

		if bestTool != nil {
			return cloneTool(bestTool)
		}
		return nil, fmt.Errorf("no active version of tool %s matches constraint %s", name, versionConstraint)
	}

	servingVersion := r.serving[name]
	if servingVersion == "" {
		return nil, fmt.Errorf("no serving version selected for tool %s", name)
	}
	servingTool, ok := versions[servingVersion]
	if !ok || !servingTool.Metadata.IsActive {
		return nil, fmt.Errorf("serving version %s of tool %s is unavailable", servingVersion, name)
	}
	return cloneTool(servingTool)
}

// ResolveDigest returns one exact active revision and never falls forward.
func (r *ToolRegistry) ResolveDigest(name, digest string) (*Tool, error) {
	versions, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	for _, tool := range versions {
		if tool.Metadata.ResourceDigest != digest {
			continue
		}
		if !tool.Metadata.IsActive {
			return nil, fmt.Errorf("tool %s revision %s is inactive", name, digest)
		}
		return cloneTool(tool)
	}
	return nil, fmt.Errorf("tool %s revision %s not found", name, digest)
}

// SetServing selects one active revision for unqualified calls.
func (r *ToolRegistry) SetServing(name, version string) error {
	versions, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("tool %s not found", name)
	}
	selected, ok := versions[version]
	if !ok {
		return fmt.Errorf("tool %s@%s not found", name, version)
	}
	if !selected.Metadata.IsActive {
		return fmt.Errorf("tool %s@%s is inactive", name, version)
	}
	for candidateVersion, candidate := range versions {
		candidate.Metadata.IsServing = candidateVersion == version
	}
	r.serving[name] = version
	return nil
}

// ListStable returns the operator-selected serving revision of each active tool.
func (r *ToolRegistry) ListStable() []*Tool {
	var result []*Tool
	for name := range r.tools {
		if t, err := r.Resolve(name, ""); err == nil {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Metadata.Name < result[j].Metadata.Name
	})
	return result
}

// Remove deactivates a specific version of a tool in the registry instead of removing it.
func (r *ToolRegistry) Remove(name, version string) {
	if versions, ok := r.tools[name]; ok {
		if t, ok := versions[version]; ok {
			t.Metadata.IsActive = false
			t.Metadata.IsServing = false
			if r.serving[name] == version {
				delete(r.serving, name)
			}
		}
	}
}

// ListAll returns all versions of all tools (including inactive ones).
func (r *ToolRegistry) ListAll() []*Tool {
	var result []*Tool
	for _, versions := range r.tools {
		for _, t := range versions {
			if cloned, err := cloneTool(t); err == nil {
				result = append(result, cloned)
			}
		}
	}
	return result
}

// ListActive returns all active versions of all tools.
func (r *ToolRegistry) ListActive() []*Tool {
	var result []*Tool
	for _, versions := range r.tools {
		for _, t := range versions {
			if t.Metadata.IsActive {
				if cloned, err := cloneTool(t); err == nil {
					result = append(result, cloned)
				}
			}
		}
	}
	return result
}

// QualifiedToolName returns a protocol-name-safe exact revision identifier.
func QualifiedToolName(name, version string) string {
	return name + ".rev_" + base64.RawURLEncoding.EncodeToString([]byte(version))
}

// ParseQualifiedToolName decodes an exact revision identifier.
func ParseQualifiedToolName(identifier string) (name, version string, ok bool) {
	index := strings.LastIndex(identifier, ".rev_")
	if index <= 0 {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(identifier[index+5:])
	if err != nil || len(decoded) == 0 {
		return "", "", false
	}
	return identifier[:index], string(decoded), true
}

// ParseToolIdentifier splits "name@version" into "name" and "version".
func ParseToolIdentifier(id string) (name, version string) {
	parts := strings.SplitN(id, "@", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}
