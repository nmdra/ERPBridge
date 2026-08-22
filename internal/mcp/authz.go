package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const roleSelectorField = "role"

type callerRoleContextKey struct{}

// WithCallerRole attaches the verified role selected for the current tool call.
func WithCallerRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, callerRoleContextKey{}, role)
}

// CallerRoleFromContext returns the verified role selected for the current call.
func CallerRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(callerRoleContextKey{}).(string)
	return role, ok && role != ""
}

// RoleAuthorizationError is returned when a caller cannot use a guarded tool.
type RoleAuthorizationError struct {
	Status int    `json:"-"`
	Reason string `json:"reason"`
}

func (e *RoleAuthorizationError) Error() string {
	return fmt.Sprintf("role authorization denied: %s", e.Reason)
}

func roleDenied(reason string) *RoleAuthorizationError {
	return &RoleAuthorizationError{Status: http.StatusForbidden, Reason: reason}
}

func roleBadRequest(reason string) *RoleAuthorizationError {
	return &RoleAuthorizationError{Status: http.StatusBadRequest, Reason: reason}
}

// RoleAuthzMiddleware verifies the role selector for guarded MCP tools before
// the cache and execution handlers run.
func (s *Server) RoleAuthzMiddleware(t *Tool) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if len(t.Spec.Security.AllowedRoles) == 0 {
				return next(ctx, request)
			}

			args, err := callArguments(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			selectedRole, selectedInArgs, err := roleSelector(args)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !selectedInArgs {
				selectedRole, selectedInArgs = CallerRoleFromContext(ctx)
			}
			if !selectedInArgs {
				err := roleDenied("a verified identity and role selector are required")
				return mcp.NewToolResultError(err.Error()), nil
			}

			authorizedContext, err := authorizeRole(ctx, t, selectedRole)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if _, roleWasInArgs := args[roleSelectorField]; roleWasInArgs {
				cleanArgs := make(map[string]any, len(args)-1)
				for key, value := range args {
					if key != roleSelectorField {
						cleanArgs[key] = value
					}
				}
				request.Params.Arguments = cleanArgs
			}
			return next(authorizedContext, request)
		}
	}
}

func callArguments(request mcp.CallToolRequest) (map[string]any, error) {
	if request.Params.Arguments == nil {
		return map[string]any{}, nil
	}
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return nil, roleBadRequest("tool arguments must be an object")
	}
	return args, nil
}

func roleSelector(args map[string]any) (string, bool, error) {
	value, exists := args[roleSelectorField]
	if !exists {
		return "", false, nil
	}
	role, ok := value.(string)
	if !ok {
		return "", false, roleBadRequest("role selector must be a string")
	}
	if _, err := NormalizeRoles([]string{role}); err != nil {
		return "", false, roleDenied("role selector is invalid")
	}
	return role, true, nil
}

func authorizeRole(ctx context.Context, tool *Tool, role string) (context.Context, error) {
	identity, ok := CallerIdentityFromContext(ctx)
	if !ok || identity.PrincipalID == "" {
		return nil, roleDenied("a verified identity is required")
	}
	if !contains(identity.Roles, role) {
		return nil, roleDenied("role is not assigned to the authenticated identity")
	}
	if !contains(tool.Spec.Security.AllowedRoles, role) {
		return nil, roleDenied("role is not allowed for this tool")
	}
	return WithCallerRole(ctx, role), nil
}

func (s *Server) authorizeDirectInvoke(ctx context.Context, tool *Tool, args map[string]any, selector string) (context.Context, map[string]any, error) {
	if len(tool.Spec.Security.AllowedRoles) == 0 {
		return ctx, args, nil
	}
	if _, exists := args[roleSelectorField]; exists {
		return nil, nil, roleBadRequest("guarded direct invokes select role with X-ERPBridge-Role")
	}
	if selector == "" {
		return nil, nil, roleDenied("X-ERPBridge-Role is required for guarded direct invokes")
	}
	if _, err := NormalizeRoles([]string{selector}); err != nil {
		return nil, nil, roleDenied("role selector is invalid")
	}
	authorizedContext, err := authorizeRole(ctx, tool, selector)
	if err != nil {
		return nil, nil, err
	}
	return authorizedContext, args, nil
}
