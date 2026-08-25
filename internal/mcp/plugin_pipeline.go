package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
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

	bindings := s.pluginRegistry.RuntimeBindingsForTool(tool.Metadata.Name, tool.Metadata.Version)
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
			Tool: ToolIdentity{
				Name:    tool.Metadata.Name,
				Version: tool.Metadata.Version,
			},
			Result: transformed,
			Config: active.Binding.Spec.Config,
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
