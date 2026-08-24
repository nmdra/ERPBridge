package mcp

import (
	"encoding/json"
	"sort"
	"sync"
)

// ActivePluginBinding is the immutable runtime view of a binding and its
// resolved active plugin resource.
type ActivePluginBinding struct {
	Binding *PluginBinding
	Plugin  *Plugin
}

// PluginRegistry holds an immutable snapshot of active bindings for runtime
// lookup. The snapshot is replaced as one unit during reconciliation.
type PluginRegistry struct {
	mu       sync.RWMutex
	bindings map[string][]*ActivePluginBinding
}

// NewPluginRegistry creates an empty active-binding snapshot.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{bindings: make(map[string][]*ActivePluginBinding)}
}

// Replace atomically replaces the active binding snapshot with a deep copy.
func (r *PluginRegistry) Replace(bindings map[string][]*ActivePluginBinding) {
	if r == nil {
		return
	}
	copySnapshot := make(map[string][]*ActivePluginBinding, len(bindings))
	for key, values := range bindings {
		cloned := make([]*ActivePluginBinding, 0, len(values))
		for _, value := range values {
			cloned = append(cloned, cloneActivePluginBinding(value))
		}
		copySnapshot[key] = cloned
	}
	r.mu.Lock()
	r.bindings = copySnapshot
	r.mu.Unlock()
}

// BindingsForTool returns copies of the active binding definitions for an
// exact tool. Runtime callers that need the endpoint use RuntimeBindingsForTool.
func (r *PluginRegistry) BindingsForTool(name, version string) []*PluginBinding {
	values := r.RuntimeBindingsForTool(name, version)
	cloned := make([]*PluginBinding, 0, len(values))
	for _, value := range values {
		if value != nil {
			cloned = append(cloned, value.Binding)
		}
	}
	return cloned
}

// SnapshotBindings returns copies of all active binding definitions.
func (r *PluginRegistry) SnapshotBindings() []*PluginBinding {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*PluginBinding
	for _, values := range r.bindings {
		for _, value := range values {
			if value != nil {
				result = append(result, clonePluginBinding(value.Binding))
			}
		}
	}
	return result
}

// RuntimeBindingsForTool returns copies of active bindings and their resolved
// plugin resources for an exact tool.
func (r *PluginRegistry) RuntimeBindingsForTool(name, version string) []*ActivePluginBinding {
	if r == nil {
		return nil
	}
	key := name + "@" + version
	r.mu.RLock()
	values := r.bindings[key]
	cloned := make([]*ActivePluginBinding, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, cloneActivePluginBinding(value))
	}
	r.mu.RUnlock()
	return cloned
}

func buildPluginBindingSnapshot(plugins []*Plugin, bindings []*PluginBinding, tools []*Tool) map[string][]*ActivePluginBinding {
	activePlugins := make(map[string]*Plugin, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Metadata.IsActive {
			continue
		}
		activePlugins[plugin.Metadata.Name+"@"+plugin.Metadata.Version] = clonePlugin(plugin)
	}
	activeTools := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil || !tool.Metadata.IsActive {
			continue
		}
		activeTools[tool.Metadata.Name+"@"+tool.Metadata.Version] = struct{}{}
	}

	snapshot := make(map[string][]*ActivePluginBinding)
	for _, binding := range bindings {
		if binding == nil || !binding.Metadata.IsActive {
			continue
		}
		pluginKey := binding.Spec.PluginRef.Name + "@" + binding.Spec.PluginRef.Version
		plugin, ok := activePlugins[pluginKey]
		if !ok || plugin == nil {
			continue
		}
		toolKey := binding.ToolKey()
		if _, ok := activeTools[toolKey]; !ok {
			continue
		}
		snapshot[toolKey] = append(snapshot[toolKey], &ActivePluginBinding{
			Binding: clonePluginBinding(binding),
			Plugin:  clonePlugin(plugin),
		})
	}
	for key := range snapshot {
		sort.SliceStable(snapshot[key], func(i, j int) bool {
			left, right := snapshot[key][i], snapshot[key][j]
			if left.Binding.Spec.Priority != right.Binding.Spec.Priority {
				return left.Binding.Spec.Priority < right.Binding.Spec.Priority
			}
			return left.Binding.Metadata.Name < right.Binding.Metadata.Name
		})
	}
	return snapshot
}

func cloneActivePluginBinding(value *ActivePluginBinding) *ActivePluginBinding {
	if value == nil {
		return nil
	}
	return &ActivePluginBinding{
		Binding: clonePluginBinding(value.Binding),
		Plugin:  clonePlugin(value.Plugin),
	}
}

func clonePluginBinding(binding *PluginBinding) *PluginBinding {
	if binding == nil {
		return nil
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return nil
	}
	var clone PluginBinding
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil
	}
	return &clone
}

func clonePlugin(plugin *Plugin) *Plugin {
	if plugin == nil {
		return nil
	}
	encoded, err := json.Marshal(plugin)
	if err != nil {
		return nil
	}
	var clone Plugin
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil
	}
	return &clone
}
