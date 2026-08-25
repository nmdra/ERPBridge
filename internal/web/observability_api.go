package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nmdra/ERPBridge/internal/bridgeclient"
	"github.com/nmdra/ERPBridge/internal/cache"
	"github.com/nmdra/ERPBridge/internal/config"
	"github.com/nmdra/ERPBridge/internal/mcp"
)

func (h *consoleHandler) health(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/mcp/health")
	if err != nil {
		writeJSON(w, http.StatusOK, HealthResponse{State: stateUnavailable})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, HealthResponse{State: upstreamState(response.StatusCode)})
		return
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusOK, HealthResponse{State: stateUnavailable})
		return
	}
	state := "degraded"
	if strings.EqualFold(payload.Status, "ok") {
		state = "healthy"
	}
	writeJSON(w, http.StatusOK, HealthResponse{State: state, Status: payload.Status})
}

func (h *consoleHandler) tools(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/apis/erpbridge.io/v1/tools")
	if err != nil {
		writeJSON(w, http.StatusOK, ToolListResponse{State: stateUnavailable, Items: []ToolProjection{}})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, ToolListResponse{State: upstreamState(response.StatusCode), Items: []ToolProjection{}})
		return
	}
	var tools []mcp.Tool
	if err := json.NewDecoder(response.Body).Decode(&tools); err != nil {
		writeJSON(w, http.StatusOK, ToolListResponse{State: stateUnavailable, Items: []ToolProjection{}})
		return
	}
	items := make([]ToolProjection, 0, len(tools))
	for _, tool := range tools {
		items = append(items, projectTool(tool))
	}
	writeJSON(w, http.StatusOK, ToolListResponse{State: stateAvailable, Items: items})
}

func (h *consoleHandler) serverInfo(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetMCPServer, "/api/info")
	if err != nil {
		writeJSON(w, http.StatusOK, ServerInfoResponse{State: stateUnavailable})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, ServerInfoResponse{State: upstreamState(response.StatusCode)})
		return
	}
	var info ServerInfoResponse
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		writeJSON(w, http.StatusOK, ServerInfoResponse{State: stateUnavailable})
		return
	}
	info.State = stateAvailable
	writeJSON(w, http.StatusOK, info)
}

func (h *consoleHandler) cache(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	ctx, ok := h.contextForRequest(w, r)
	if !ok {
		return
	}
	response, err := h.upstreamRequest(r, ctx, bridgeclient.TargetServer, "/api/cache/stats")
	if err != nil {
		writeJSON(w, http.StatusOK, CacheResponse{State: stateUnavailable})
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeJSON(w, http.StatusOK, CacheResponse{State: upstreamState(response.StatusCode)})
		return
	}
	var payload struct {
		Stats cache.Stats `json:"stats"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusOK, CacheResponse{State: stateUnavailable})
		return
	}
	writeJSON(w, http.StatusOK, CacheResponse{
		State: stateAvailable,
		Stats: &CacheStatsProjection{ExactKeys: payload.Stats.ExactKeys, RedisMemory: payload.Stats.RedisMemory},
	})
}

func (h *consoleHandler) contextForRequest(w http.ResponseWriter, r *http.Request) (config.Context, bool) {
	name := r.URL.Query().Get("context")
	ctx, ok := h.context(name)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "context_not_found", "the selected context is not configured")
		return config.Context{}, false
	}
	return ctx, true
}

func (h *consoleHandler) upstreamRequest(r *http.Request, ctx config.Context, target bridgeclient.Target, path string) (*http.Response, error) {
	return h.upstreamRequestWithHeaders(r, ctx, target, path, nil, http.Header{"Accept": []string{"application/json"}})
}

func (h *consoleHandler) upstreamRequestWithHeaders(r *http.Request, ctx config.Context, target bridgeclient.Target, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	client, err := bridgeclient.New(ctx, h.tokenOverride)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(path, "/api/logs/") {
		client.MaxResponseBytes = maxLogResponseBytes
	}
	if path == "/metrics" {
		client.MaxResponseBytes = maxMetricsBytes
	}
	requestContext, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	response, err := client.Do(requestContext, target, http.MethodGet, path, body, headers)
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &cancelOnCloseBody{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancelOnce sync.Once
	cancel     context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancelOnce.Do(b.cancel)
	return err
}

func projectTool(tool mcp.Tool) ToolProjection {
	projection := ToolProjection{
		Name:         tool.Metadata.Name,
		Version:      tool.Metadata.Version,
		Module:       tool.Metadata.Module,
		Status:       tool.Metadata.Status,
		Active:       tool.Metadata.IsActive,
		Description:  tool.Spec.Description.Short,
		Method:       strings.ToUpper(tool.Spec.Execution.Method),
		EndpointPath: safeEndpointPath(tool.Spec.Execution.Endpoint),
		ResponsePath: tool.Spec.Execution.ResponsePath,
		AllowedRoles: append([]string(nil), tool.Spec.Security.AllowedRoles...),
		Manifest:     projectToolManifest(tool),
	}
	if tool.Spec.Cache != nil {
		projection.Cache = &CacheProjection{
			Enabled:    tool.Spec.Cache.Enabled,
			TTLSeconds: tool.Spec.Cache.TTLSeconds,
			IsReadOnly: tool.Spec.Cache.IsReadOnly,
		}
	}
	if tool.Spec.Lifecycle != nil {
		projection.Lifecycle = &LifecycleProjection{
			Status:       tool.Spec.Lifecycle.Status,
			DeprecatedAt: tool.Spec.Lifecycle.DeprecatedAt,
			SunsetAt:     tool.Spec.Lifecycle.SunsetAt,
			Replacement:  tool.Spec.Lifecycle.Replacement,
		}
	}
	return projection
}

func projectToolManifest(tool mcp.Tool) *ToolManifestProjection {
	required := make(map[string]struct{}, len(tool.Spec.InputSchema.Required))
	for _, name := range tool.Spec.InputSchema.Required {
		required[name] = struct{}{}
	}
	fieldNames := make([]string, 0, len(tool.Spec.InputSchema.Properties))
	for name := range tool.Spec.InputSchema.Properties {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	fields := make([]ToolInputFieldProjection, 0, len(fieldNames))
	for _, name := range fieldNames {
		property := tool.Spec.InputSchema.Properties[name]
		fields = append(fields, ToolInputFieldProjection{
			Name:        name,
			Type:        property.Type,
			Description: property.Description,
			Enum:        append([]string(nil), property.Enum...),
			Required:    hasRequired(required, name),
		})
	}
	manifest := &ToolManifestProjection{
		APIVersion: tool.APIVersion,
		Kind:       tool.Kind,
		Description: ToolDescriptionProjection{
			Short:        tool.Spec.Description.Short,
			WhenToUse:    append([]string(nil), tool.Spec.Description.WhenToUse...),
			WhenNotToUse: append([]string(nil), tool.Spec.Description.WhenNotToUse...),
			Examples:     append([]string(nil), tool.Spec.Description.Examples...),
		},
		InputType:   tool.Spec.InputSchema.Type,
		InputFields: fields,
		OutputType:  schemaType(tool.Spec.OutputSchema),
		Execution: ToolExecutionProjection{
			Type:         tool.Spec.Execution.Type,
			Method:       strings.ToUpper(tool.Spec.Execution.Method),
			EndpointPath: safeEndpointPath(tool.Spec.Execution.Endpoint),
			ResponsePath: tool.Spec.Execution.ResponsePath,
			Mapping:      cloneStringMap(tool.Spec.Execution.Mapping),
		},
		Security: ToolSecurityProjection{
			AuthType:     tool.Spec.Security.AuthType,
			AllowedRoles: append([]string(nil), tool.Spec.Security.AllowedRoles...),
		},
	}
	if tool.Spec.Routing != nil {
		manifest.Routing = &ToolRoutingProjection{
			Priority:    tool.Spec.Routing.Priority,
			Signals:     append([]string(nil), tool.Spec.Routing.Signals...),
			AntiSignals: append([]string(nil), tool.Spec.Routing.AntiSignals...),
		}
	}
	return manifest
}

func hasRequired(required map[string]struct{}, name string) bool {
	_, ok := required[name]
	return ok
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func schemaType(schema *any) string {
	if schema == nil {
		return ""
	}
	object, ok := (*schema).(map[string]any)
	if !ok {
		return "defined"
	}
	if value, ok := object["type"].(string); ok {
		return value
	}
	return "defined"
}

func safeEndpointPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Path == "" && strings.HasPrefix(raw, "/") {
		return strings.SplitN(raw, "?", 2)[0]
	}
	return parsed.Path
}

func upstreamState(status int) string {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "unauthorized"
	}
	return stateUnavailable
}
