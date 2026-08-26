package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const (
	pluginAppliedStatus = "applied"
	pluginVersionField  = "version"
	queryTrueValue      = "true"
)

func writeReconciliationPending(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		statusKey: "pending",
		"message": "desired state is stored and awaits successful reconciliation",
	})
}

// handlePluginAPI serves the Plugin control-plane resource.
func (s *Server) handlePluginAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePluginApply(w, r)
	case http.MethodGet:
		s.handlePluginList(w, r)
	case http.MethodDelete:
		s.handlePluginDelete(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handlePluginBindingAPI serves the PluginBinding control-plane resource.
func (s *Server) handlePluginBindingAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePluginBindingApply(w, r)
	case http.MethodGet:
		s.handlePluginBindingList(w, r)
	case http.MethodDelete:
		s.handlePluginBindingDelete(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func decodeStrictJSON(r io.Reader, target any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON documents are not allowed")
		}
		return err
	}
	return nil
}

func (s *Server) handlePluginApply(w http.ResponseWriter, r *http.Request) {
	var plugin Plugin
	if err := decodeStrictJSON(r.Body, &plugin); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := plugin.Validate(); err != nil {
		http.Error(w, "invalid plugin: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := credentialedPluginAdmission(r.Context(), &plugin); err != nil {
		http.Error(w, "invalid plugin: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	affected, err := s.bindingsReferencingPlugin(plugin.Metadata.Name, plugin.Metadata.Version)
	if err != nil {
		http.Error(w, "failed to inspect plugin bindings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.flushPluginBindingTargets(r.Context(), affected); err != nil {
		http.Error(w, "plugin cache invalidation failed", http.StatusInternalServerError)
		return
	}
	plugin.Metadata.IsActive = true
	if err := s.store.SavePlugin(&plugin); err != nil {
		http.Error(w, "failed to save plugin: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		statusKey:          pluginAppliedStatus,
		"name":             plugin.Metadata.Name,
		pluginVersionField: plugin.Metadata.Version,
	})
}

func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	plugins, err := s.store.ListPlugins()
	if err != nil {
		http.Error(w, "failed to list plugins: "+err.Error(), http.StatusInternalServerError)
		return
	}
	nameFilter := r.URL.Query().Get("name")
	versionFilter := r.URL.Query().Get("version")
	filtered := make([]*Plugin, 0, len(plugins))
	for _, plugin := range plugins {
		if nameFilter != "" && plugin.Metadata.Name != nameFilter {
			continue
		}
		if versionFilter != "" && plugin.Metadata.Version != versionFilter {
			continue
		}
		filtered = append(filtered, plugin)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filtered)
}

func (s *Server) handlePluginDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	version := r.URL.Query().Get("version")
	if name == "" || version == "" {
		http.Error(w, "missing name or version parameter", http.StatusBadRequest)
		return
	}
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	affected, err := s.bindingsReferencingPlugin(name, version)
	if err != nil {
		http.Error(w, "failed to inspect plugin bindings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.flushPluginBindingTargets(r.Context(), affected); err != nil {
		http.Error(w, "plugin cache invalidation failed", http.StatusInternalServerError)
		return
	}

	var deleteErr error
	if r.URL.Query().Get("hard") == queryTrueValue {
		deleteErr = s.store.HardDeletePlugin(name, version)
	} else {
		deleteErr = s.store.DeletePlugin(name, version)
	}
	if deleteErr != nil {
		if errors.Is(deleteErr, ErrPluginHasActiveBindings) {
			http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
			return
		}
		http.Error(w, "failed to delete plugin: "+deleteErr.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pluginEndpointAllowlisted(endpoint string) bool {
	for _, allowed := range strings.Split(os.Getenv("PLUGIN_ENDPOINT_ALLOWLIST"), ",") {
		if strings.EqualFold(strings.TrimSpace(allowed), endpoint) {
			return true
		}
	}
	return false
}

func rawBindingRuntimeAdmission(plugin *Plugin, tool *Tool) error {
	if plugin == nil || tool == nil {
		return errors.New("raw response binding references are unavailable")
	}
	if strings.TrimSpace(os.Getenv(authTokenEnv)) == "" {
		return fmt.Errorf("raw response bindings require %s to be configured", authTokenEnv)
	}
	if tool.Handler != nil || tool.Spec.Execution.Type != "http" || strings.TrimSpace(tool.Spec.Execution.Endpoint) == "" {
		return errors.New("raw response bindings require an active HTTP tool")
	}
	if !hasObjectOutputSchema(tool) {
		return errors.New("raw response bindings require an object output schema")
	}
	endpoint, err := pluginEndpointHostPort(plugin.Spec.Endpoint)
	if err != nil {
		return fmt.Errorf("normalize plugin endpoint: %w", err)
	}
	if !pluginEndpointAllowlisted(endpoint) {
		return fmt.Errorf("plugin endpoint %q is not in PLUGIN_ENDPOINT_ALLOWLIST", endpoint)
	}
	return nil
}

func hasObjectOutputSchema(tool *Tool) bool {
	if tool == nil || tool.Spec.OutputSchema == nil {
		return false
	}
	encoded, err := json.Marshal(*tool.Spec.OutputSchema)
	if err != nil {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return false
	}
	value, ok := schema["type"].(string)
	return ok && value == schemaTypeObject
}

func credentialedPluginAdmission(ctx context.Context, plugin *Plugin) error {
	if plugin.Spec.Auth == nil {
		return nil
	}
	if strings.TrimSpace(os.Getenv(authTokenEnv)) == "" {
		return fmt.Errorf("credentialed plugins require %s to be configured", authTokenEnv)
	}
	identity, ok := CallerIdentityFromContext(ctx)
	if !ok || !identity.IsAdmin {
		return errors.New("credentialed plugins require an authenticated admin")
	}
	endpoint, err := pluginEndpointHostPort(plugin.Spec.Endpoint)
	if err != nil {
		return fmt.Errorf("normalize plugin endpoint: %w", err)
	}
	if pluginEndpointAllowlisted(endpoint) {
		return nil
	}
	return fmt.Errorf("plugin endpoint %q is not in PLUGIN_ENDPOINT_ALLOWLIST", endpoint)
}

func (s *Server) handlePluginBindingApply(w http.ResponseWriter, r *http.Request) {
	var binding PluginBinding
	if err := decodeStrictJSON(r.Body, &binding); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := binding.Validate(); err != nil {
		http.Error(w, "invalid plugin binding: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	if err := s.validatePluginBindingReferences(r.Context(), &binding); err != nil {
		http.Error(w, "invalid plugin binding: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	var affected []*PluginBinding
	if existing, err := s.store.GetPluginBinding(binding.Metadata.Name); err == nil {
		affected = append(affected, existing)
	} else if !errors.Is(err, ErrPluginBindingNotFound) {
		http.Error(w, "failed to inspect existing plugin binding: "+err.Error(), http.StatusInternalServerError)
		return
	}
	binding.Metadata.IsActive = true
	affected = append(affected, &binding)
	if err := s.flushPluginBindingTargets(r.Context(), affected); err != nil {
		http.Error(w, "plugin cache invalidation failed", http.StatusInternalServerError)
		return
	}
	if err := s.store.SavePluginBinding(&binding); err != nil {
		http.Error(w, "failed to save plugin binding: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		statusKey: pluginAppliedStatus,
		"name":    binding.Metadata.Name,
	})
}

func (s *Server) handlePluginBindingList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	bindings, err := s.store.ListPluginBindings()
	if err != nil {
		http.Error(w, "failed to list plugin bindings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	query := r.URL.Query()
	filtered := make([]*PluginBinding, 0, len(bindings))
	for _, binding := range bindings {
		if value := query.Get("name"); value != "" && binding.Metadata.Name != value {
			continue
		}
		if value := query.Get("pluginName"); value != "" && binding.Spec.PluginRef.Name != value {
			continue
		}
		if value := query.Get("pluginVersion"); value != "" && binding.Spec.PluginRef.Version != value {
			continue
		}
		if value := query.Get("toolName"); value != "" && binding.Spec.ToolRef.Name != value {
			continue
		}
		if value := query.Get("toolVersion"); value != "" && binding.Spec.ToolRef.Version != value {
			continue
		}
		filtered = append(filtered, binding)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filtered)
}

func (s *Server) handlePluginBindingDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "missing name parameter", http.StatusBadRequest)
		return
	}
	if s.store == nil {
		http.Error(w, "store not available", http.StatusInternalServerError)
		return
	}
	s.pluginLifecycleMu.Lock()
	defer s.pluginLifecycleMu.Unlock()

	binding, err := s.store.GetPluginBinding(name)
	if err != nil {
		http.Error(w, "failed to get plugin binding: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.flushPluginBindingTargets(r.Context(), []*PluginBinding{binding}); err != nil {
		http.Error(w, "plugin cache invalidation failed", http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("hard") == queryTrueValue {
		err = s.store.HardDeletePluginBinding(name)
	} else {
		err = s.store.DeletePluginBinding(name)
	}
	if err != nil {
		http.Error(w, "failed to delete plugin binding: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.reconcileLocked(r.Context()); err != nil {
		writeReconciliationPending(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) validatePluginBindingReferences(ctx context.Context, binding *PluginBinding) error {
	plugin, err := s.store.GetPlugin(binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version)
	if err != nil {
		return fmt.Errorf("plugin %s@%s is not active", binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version)
	}
	if !plugin.Metadata.IsActive {
		return fmt.Errorf("plugin %s@%s is not active", binding.Spec.PluginRef.Name, binding.Spec.PluginRef.Version)
	}
	tool, err := s.store.Get(binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	if err != nil {
		return fmt.Errorf("tool %s@%s is not active", binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	}
	if !tool.Metadata.IsActive {
		return fmt.Errorf("tool %s@%s is not active", binding.Spec.ToolRef.Name, binding.Spec.ToolRef.Version)
	}
	if binding.Spec.Phase == PluginPhaseRawResponse {
		if err := rawBindingRuntimeAdmission(plugin, tool); err != nil {
			return err
		}
		identity, ok := CallerIdentityFromContext(ctx)
		if !ok || !identity.IsAdmin {
			return errors.New("raw response bindings require an authenticated admin")
		}
	}
	return nil
}

func (s *Server) bindingsReferencingPlugin(name, version string) ([]*PluginBinding, error) {
	bindings, err := s.store.ListPluginBindings()
	if err != nil {
		return nil, err
	}
	result := make([]*PluginBinding, 0)
	for _, binding := range bindings {
		if binding.Spec.PluginRef.Name == name && binding.Spec.PluginRef.Version == version {
			result = append(result, binding)
		}
	}
	return result, nil
}

func (s *Server) flushPluginBindingTargets(ctx context.Context, bindingGroups ...[]*PluginBinding) error {
	if s.cache == nil {
		return nil
	}
	tools := make(map[string]struct{})
	for _, group := range bindingGroups {
		for _, binding := range group {
			if binding == nil || binding.Spec.ToolRef.Name == "" {
				continue
			}
			tools[binding.Spec.ToolRef.Name] = struct{}{}
		}
	}
	var firstErr error
	for toolName := range tools {
		if _, err := s.cache.FlushToolInternal(ctx, toolName); err != nil {
			s.log.Warn("plugin lifecycle cache flush failed", slog.String("tool_name", toolName), slog.String("error", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
