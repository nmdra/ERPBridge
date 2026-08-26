package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nmdra/ERPBridge/internal/connector"
)

const (
	pluginProcessingFailureMessage = "plugin processing failed"
	pluginErrorField               = "error"
)

// ErrPluginProcessingFailed is the only error exposed when a binding uses the
// fail policy. It deliberately contains no endpoint, payload, or plugin body.
var ErrPluginProcessingFailed = errors.New(pluginProcessingFailureMessage)

func (s *Server) executeToolCall(ctx context.Context, tool *Tool, args map[string]any) (*mcp.CallToolResult, error) {
	result, err := s.executeTool(ctx, tool, args)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(result.Result)
	return mcp.NewToolResultText(string(encoded)), nil
}

func (s *Server) executeTool(ctx context.Context, tool *Tool, args map[string]any) (*ToolResult, error) {
	rawBindings := s.pluginRegistry.RuntimeBindingsForToolPhase(tool.Metadata.Name, tool.Metadata.Version, PluginPhaseRawResponse)
	afterBindings := s.pluginRegistry.RuntimeBindingsForToolPhase(tool.Metadata.Name, tool.Metadata.Version, PluginPhaseAfterResponse)
	if len(rawBindings) > 0 {
		return s.executeRawTool(ctx, tool, args, rawBindings, afterBindings)
	}

	result, err := tool.Execute(ctx, args, s.connector)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("tool returned no result")
	}
	if result.IsError {
		return result, nil
	}
	if validationErr := tool.ValidateResult(result.Result); validationErr != nil {
		result.Error = fmt.Sprintf("response validation failed: %v", validationErr)
		result.IsError = true
		return result, nil
	}
	return s.processAfterResponseBindings(ctx, tool, result, afterBindings)
}

func (s *Server) executeRawTool(ctx context.Context, tool *Tool, args map[string]any, rawBindings, afterBindings []*ActivePluginBinding) (*ToolResult, error) {
	first := rawBindings[0]
	if tool.Handler != nil || tool.Spec.Execution.Type != pluginSchemeHTTP || !hasObjectOutputSchema(tool) {
		return s.rawProcessingFailure(ctx, tool, first, nil, afterBindings, errors.New("raw response binding is not valid for this tool"))
	}
	captured, err := tool.CallERP(ctx, args, s.connector, connector.CallOptions{PreserveErrorResponses: true})
	if err != nil {
		return s.rawProcessingFailure(ctx, tool, first, nil, afterBindings, err)
	}
	if captured == nil {
		return s.rawProcessingFailure(ctx, tool, first, nil, afterBindings, errors.New("ERP response is unavailable"))
	}

	rawResponse := rawResponseFromERP(captured)
	var transformed any
	var lastBinding *ActivePluginBinding
	for _, active := range rawBindings {
		if active == nil || active.Binding == nil || active.Plugin == nil {
			return s.rawProcessingFailure(ctx, tool, active, captured, afterBindings, ErrPluginProcessingFailed)
		}
		if s.pluginClient == nil {
			return s.rawProcessingFailure(ctx, tool, active, captured, afterBindings, errors.New("plugin client unavailable"))
		}
		response, processErr := s.pluginClient.Process(ctx, active.Plugin, PluginInvocation{
			ProtocolVersion: PluginProtocolVersion,
			Tool:            ToolIdentity{Name: tool.Metadata.Name, Version: tool.Metadata.Version},
			RawResponse:     rawResponse,
			Config:          active.Binding.Spec.Config,
		})
		if processErr != nil || response == nil {
			return s.rawProcessingFailure(ctx, tool, active, captured, afterBindings, processErr)
		}
		transformed = response.Result
		lastBinding = active
		rawResponse = &PluginRawResponse{
			Status:      captured.Status,
			ContentType: "application/json",
			Body: PluginRawBody{
				Encoding: PluginRawBodyEncodingJSON,
				Value:    transformed,
			},
		}
	}

	result := &ToolResult{Result: transformed, IsError: captured.Status < 200 || captured.Status >= 300}
	if result.IsError {
		return result, nil
	}
	if tool.Spec.Execution.ResponsePath != "" {
		result.Result, err = resolveResponsePath(result.Result, tool.Spec.Execution.ResponsePath)
		if err != nil {
			return s.rawProcessingFailure(ctx, tool, lastBinding, captured, afterBindings, err)
		}
	}
	if err := tool.ValidateResult(result.Result); err != nil {
		return s.rawProcessingFailure(ctx, tool, lastBinding, captured, afterBindings, err)
	}
	return s.processAfterResponseBindings(ctx, tool, result, afterBindings)
}

func rawResponseFromERP(response *ERPResponse) *PluginRawResponse {
	raw := &PluginRawResponse{
		Status:      response.Status,
		ContentType: response.ContentType,
		Body: PluginRawBody{
			Encoding: PluginRawBodyEncodingBase64,
			Value:    base64.StdEncoding.EncodeToString(response.Body),
		},
	}
	if isJSONContentType(response.ContentType) && len(response.Body) > 0 {
		var value any
		if json.Unmarshal(response.Body, &value) == nil {
			raw.Body.Encoding = PluginRawBodyEncodingJSON
			raw.Body.Value = value
		}
	}
	return raw
}

func isJSONContentType(contentType string) bool {
	return strings.EqualFold(contentType, "application/json") || strings.HasSuffix(strings.ToLower(contentType), "+json") || strings.EqualFold(contentType, "text/json")
}

func (s *Server) processAfterResponseBindings(ctx context.Context, tool *Tool, result *ToolResult, bindings []*ActivePluginBinding) (*ToolResult, error) {
	if len(bindings) == 0 {
		return result, nil
	}
	original := result.Result
	transformed := original
	var lastSuccessfulBinding *ActivePluginBinding
	for _, active := range bindings {
		if active == nil || active.Binding == nil || active.Plugin == nil {
			if err := s.handlePluginFailure(tool, active, ErrPluginProcessingFailed); err != nil {
				return nil, err
			}
			result.Result = original
			return result, nil
		}
		if s.pluginClient == nil {
			if err := s.handlePluginFailure(tool, active, errors.New("plugin client unavailable")); err != nil {
				return nil, err
			}
			result.Result = original
			return result, nil
		}
		response, err := s.pluginClient.Process(ctx, active.Plugin, PluginInvocation{
			ProtocolVersion: PluginProtocolVersion,
			Tool:            ToolIdentity{Name: tool.Metadata.Name, Version: tool.Metadata.Version},
			Result:          transformed,
			Config:          active.Binding.Spec.Config,
		})
		if err != nil || response == nil {
			if failureErr := s.handlePluginFailure(tool, active, err); failureErr != nil {
				return nil, failureErr
			}
			result.Result = original
			return result, nil
		}
		transformed = response.Result
		lastSuccessfulBinding = active
	}
	if err := tool.ValidateResult(transformed); err != nil {
		if failureErr := s.handlePluginFailure(tool, lastSuccessfulBinding, err); failureErr != nil {
			return nil, failureErr
		}
		result.Result = original
		return result, nil
	}
	result.Result = transformed
	return result, nil
}

func (s *Server) rawProcessingFailure(ctx context.Context, tool *Tool, active *ActivePluginBinding, captured *ERPResponse, afterBindings []*ActivePluginBinding, cause error) (*ToolResult, error) {
	if failureErr := s.handlePluginFailure(tool, active, cause); failureErr != nil {
		return nil, failureErr
	}
	if captured != nil && captured.Status >= 200 && captured.Status < 300 {
		fallback, fallbackErr := normalizeCapturedERPResponse(tool, captured)
		if fallbackErr == nil && fallback != nil && !fallback.IsError {
			return s.processAfterResponseBindings(ctx, tool, fallback, afterBindings)
		}
	}
	return &ToolResult{Error: "ERP response could not be processed", IsError: true}, nil
}

func normalizeCapturedERPResponse(tool *Tool, captured *ERPResponse) (*ToolResult, error) {
	var value any
	if err := json.Unmarshal(captured.Body, &value); err != nil {
		return nil, err
	}
	if tool.Spec.Execution.ResponsePath != "" {
		var err error
		value, err = resolveResponsePath(value, tool.Spec.Execution.ResponsePath)
		if err != nil {
			return nil, err
		}
	}
	if err := tool.ValidateResult(value); err != nil {
		return &ToolResult{Result: value, Error: fmt.Sprintf("response validation failed: %v", err), IsError: true}, nil
	}
	return &ToolResult{Result: value}, nil
}

func (s *Server) handlePluginFailure(tool *Tool, active *ActivePluginBinding, _ error) error {
	if active == nil || active.Binding == nil {
		return ErrPluginProcessingFailed
	}
	pluginName := active.Binding.Spec.PluginRef.Name
	pluginVersion := active.Binding.Spec.PluginRef.Version
	if active.Plugin != nil {
		pluginName = active.Plugin.Metadata.Name
		pluginVersion = active.Plugin.Metadata.Version
	}
	s.log.Warn("external plugin processing failed",
		slog.String("plugin_name", pluginName),
		slog.String("plugin_version", pluginVersion),
		slog.String("tool_name", tool.Metadata.Name),
		slog.String("tool_version", tool.Metadata.Version),
		slog.String("binding_name", active.Binding.Metadata.Name),
		slog.String("failure_policy", active.Binding.EffectiveFailurePolicy()),
	)
	if active.Binding.EffectiveFailurePolicy() == PluginFailurePolicyFail {
		return ErrPluginProcessingFailed
	}
	return nil
}
