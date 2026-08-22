package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/nmdra/ERPBridge/internal/logger"
)

const (
	authTokenEnv      = "API_AUTH_TOKEN" // #nosec G101 -- this is an environment-variable name.
	authAdminRolesEnv = "API_AUTH_ADMIN_ROLES"
)

// AuthHandler protects an HTTP route with the configured bearer credential.
// Authentication is opt-in: an unset or empty API_AUTH_TOKEN keeps the
// existing open route behavior while still allowing authorization middleware
// to fail closed for guarded tools.
func (s *Server) AuthHandler(next http.Handler, scope string, adminOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, status, err := s.authenticateHTTP(r, scope, adminOnly)
		if status != 0 {
			if s.log != nil {
				s.log.Warn("HTTP authentication denied",
					slog.Int("status", status),
					slog.String("path", r.URL.Path),
					slog.Any("headers", logger.RedactHeaders(r.Header)),
				)
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		if err != nil {
			s.log.Error("authentication configuration failed", slog.String("error", err.Error()))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, withAuthContext(ctx, r)) //nolint:contextcheck // authenticated context inherits the request context.
	})
}

func (s *Server) authenticateHTTP(r *http.Request, scope string, adminOnly bool) (context.Context, int, error) {
	adminToken, enabled := os.LookupEnv(authTokenEnv)
	if !enabled || adminToken == "" {
		s.authWarnOnce.Do(func() {
			if s.log != nil {
				s.log.Warn("HTTP authentication is disabled; protected routes are open")
			}
		})
		return r.Context(), 0, nil
	}

	adminRoles, err := configuredAdminRoles()
	if err != nil {
		return nil, 0, err
	}
	presented, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		return nil, http.StatusUnauthorized, nil
	}

	identity := CallerIdentity{}
	if secureTokenEqual(presented, adminToken) {
		identity = CallerIdentity{PrincipalID: "admin", Roles: adminRoles, IsAdmin: true}
	} else {
		if s.store == nil {
			return nil, http.StatusUnauthorized, nil
		}
		record, lookupErr := s.store.LookupToken(presented)
		if lookupErr != nil {
			return nil, http.StatusUnauthorized, nil
		}
		identity = CallerIdentity{PrincipalID: record.ID, Roles: record.Roles}
		if adminOnly {
			return nil, http.StatusForbidden, nil
		}
		if scope != "" && !contains(record.Scopes, scope) {
			return nil, http.StatusForbidden, nil
		}
	}

	if adminOnly {
		return WithRateLimitPrincipal(WithCallerIdentity(r.Context(), identity), identity.PrincipalID), 0, nil
	}
	return WithRateLimitPrincipal(WithCallerIdentity(r.Context(), identity), identity.PrincipalID), 0, nil
}

func configuredAdminRoles() ([]string, error) {
	value := strings.TrimSpace(os.Getenv(authAdminRolesEnv))
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	roles := make([]string, 0, len(parts))
	for _, part := range parts {
		roles = append(roles, strings.TrimSpace(part))
	}
	return NormalizeRoles(roles)
}

func configuredCORSOrigins() []string {
	value := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if value == "" {
		if token, enabled := os.LookupEnv(authTokenEnv); enabled && token != "" {
			return nil
		}
		return []string{"*"}
	}
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func allowedCORSOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range configuredCORSOrigins() {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func isAllowedMCPPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" && allowedCORSOrigin(r.Header.Get("Origin"))
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func secureTokenEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Server) handleTokenAPI(w http.ResponseWriter, r *http.Request) {
	ctx, status, err := s.authenticateHTTP(r, "", true)
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if err != nil {
		s.log.Error("authentication configuration failed", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	r = withAuthContext(ctx, r) //nolint:contextcheck // authenticated context inherits the request context.

	if s.store == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/tokens")
	switch {
	case r.Method == http.MethodPost && path == "":
		var request TokenCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		record, raw, err := s.store.CreateToken(request)
		if err != nil {
			http.Error(w, "invalid token: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			TokenRecord
			Token string `json:"token"`
		}{TokenRecord: record, Token: raw})
	case r.Method == http.MethodGet && path == "":
		records, err := s.store.ListTokens()
		if err != nil {
			http.Error(w, "failed to list tokens: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(records)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/") && len(path) > 1 && !strings.Contains(path[1:], "/"):
		if err := s.store.RevokeToken(strings.TrimPrefix(path, "/")); err != nil {
			if errors.Is(err, ErrTokenNotFound) {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
			http.Error(w, "failed to revoke token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func withAuthContext(ctx context.Context, r *http.Request) *http.Request {
	return r.WithContext(ctx)
}
