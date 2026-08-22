package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	tokenPrefix = "erpbt_"
	tokenBytes  = 32
	maxRoles    = 32
	scopeMCP    = "mcp"
)

var (
	rolePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

	// ErrTokenNotFound indicates that a presented or requested token does not exist.
	ErrTokenNotFound = errors.New("token not found")
	// ErrTokenRevoked indicates that a token was explicitly revoked.
	ErrTokenRevoked = errors.New("token revoked")
	// ErrTokenExpired indicates that a token is past its configured expiry.
	ErrTokenExpired = errors.New("token expired")
)

// CallerIdentity is the authenticated identity attached to a request context.
// Roles are copied when they enter and leave a context so callers cannot mutate
// authorization state through a shared slice.
type CallerIdentity struct {
	PrincipalID string
	Roles       []string
	IsAdmin     bool
}

type callerIdentityContextKey struct{}

// WithCallerIdentity attaches an immutable copy of identity to ctx.
func WithCallerIdentity(ctx context.Context, identity CallerIdentity) context.Context {
	identity.Roles = append([]string(nil), identity.Roles...)
	return context.WithValue(ctx, callerIdentityContextKey{}, identity)
}

// CallerIdentityFromContext returns a copy of the authenticated identity.
func CallerIdentityFromContext(ctx context.Context) (CallerIdentity, bool) {
	identity, ok := ctx.Value(callerIdentityContextKey{}).(CallerIdentity)
	if !ok {
		return CallerIdentity{}, false
	}
	identity.Roles = append([]string(nil), identity.Roles...)
	return identity, true
}

// NormalizeRoles validates and returns a sorted, independent role list.
func NormalizeRoles(roles []string) ([]string, error) {
	if len(roles) > maxRoles {
		return nil, fmt.Errorf("roles must contain at most %d entries", maxRoles)
	}

	result := append([]string(nil), roles...)
	seen := make(map[string]struct{}, len(result))
	for _, role := range result {
		if !rolePattern.MatchString(role) {
			return nil, fmt.Errorf("invalid role %q", role)
		}
		if _, exists := seen[role]; exists {
			return nil, fmt.Errorf("duplicate role %q", role)
		}
		seen[role] = struct{}{}
	}
	sort.Strings(result)
	return result, nil
}

// TokenCreateRequest contains the metadata assigned to a newly generated token.
type TokenCreateRequest struct {
	Name      string
	Scopes    []string
	Roles     []string
	ExpiresAt *time.Time
}

// TokenRecord contains persisted token metadata. TokenHash is internal and is
// never serialized or returned by API handlers.
type TokenRecord struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	TokenHash string     `json:"-"`
	Scopes    []string   `json:"scopes"`
	Roles     []string   `json:"roles"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// CreateToken generates an opaque bearer token and persists only its hash.
func (s *Store) CreateToken(request TokenCreateRequest) (TokenRecord, string, error) {
	if strings.TrimSpace(request.Name) == "" {
		return TokenRecord{}, "", errors.New("token name is required")
	}
	if len(request.Name) > 128 {
		return TokenRecord{}, "", errors.New("token name is too long")
	}
	roles, err := NormalizeRoles(request.Roles)
	if err != nil {
		return TokenRecord{}, "", err
	}
	scopes, err := normalizeScopes(request.Scopes)
	if err != nil {
		return TokenRecord{}, "", err
	}
	if request.ExpiresAt != nil && !request.ExpiresAt.After(time.Now()) {
		return TokenRecord{}, "", errors.New("token expiry must be in the future")
	}

	rawBytes := make([]byte, tokenBytes)
	if _, err := rand.Read(rawBytes); err != nil {
		return TokenRecord{}, "", fmt.Errorf("generate token: %w", err)
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return TokenRecord{}, "", fmt.Errorf("generate token id: %w", err)
	}

	rawToken := tokenPrefix + hex.EncodeToString(rawBytes)
	tokenHash := hashToken(rawToken)
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return TokenRecord{}, "", fmt.Errorf("marshal token roles: %w", err)
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return TokenRecord{}, "", fmt.Errorf("marshal token scopes: %w", err)
	}

	now := time.Now().UTC()
	var expiresAt any
	if request.ExpiresAt != nil {
		expiresAt = request.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.Exec(`
		INSERT INTO api_tokens (id, name, token_hash, scopes, roles, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, tokenPrefix+hex.EncodeToString(idBytes), request.Name, tokenHash, string(scopesJSON), string(rolesJSON), expiresAt, now.Format(time.RFC3339Nano))
	if err != nil {
		return TokenRecord{}, "", fmt.Errorf("save token: %w", err)
	}

	record := TokenRecord{
		ID:        tokenPrefix + hex.EncodeToString(idBytes),
		Name:      request.Name,
		Scopes:    scopes,
		Roles:     roles,
		ExpiresAt: cloneTime(request.ExpiresAt),
		CreatedAt: now,
	}
	return record, rawToken, nil
}

// LookupToken resolves an active token by hashing the presented bearer value.
func (s *Store) LookupToken(rawToken string) (TokenRecord, error) {
	var record TokenRecord
	var scopesJSON, rolesJSON string
	var expiresAt, revokedAt sql.NullString
	var createdAt string
	err := s.db.QueryRow(`
		SELECT id, name, token_hash, scopes, roles, expires_at, revoked_at, created_at
		FROM api_tokens WHERE token_hash = ?
	`, hashToken(rawToken)).Scan(
		&record.ID, &record.Name, &record.TokenHash, &scopesJSON, &rolesJSON,
		&expiresAt, &revokedAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenRecord{}, ErrTokenNotFound
	}
	if err != nil {
		return TokenRecord{}, fmt.Errorf("lookup token: %w", err)
	}

	if revokedAt.Valid {
		return TokenRecord{}, ErrTokenRevoked
	}
	if expiresAt.Valid {
		expires, parseErr := time.Parse(time.RFC3339Nano, expiresAt.String)
		if parseErr != nil {
			return TokenRecord{}, fmt.Errorf("parse token expiry: %w", parseErr)
		}
		record.ExpiresAt = &expires
		if !expires.After(time.Now()) {
			return TokenRecord{}, ErrTokenExpired
		}
	}
	if err := json.Unmarshal([]byte(scopesJSON), &record.Scopes); err != nil {
		return TokenRecord{}, fmt.Errorf("decode token scopes: %w", err)
	}
	if err := json.Unmarshal([]byte(rolesJSON), &record.Roles); err != nil {
		return TokenRecord{}, fmt.Errorf("decode token roles: %w", err)
	}
	if createdAt != "" {
		created, parseErr := time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			return TokenRecord{}, fmt.Errorf("parse token creation time: %w", parseErr)
		}
		record.CreatedAt = created
	}
	record.TokenHash = ""
	return record, nil
}

// ListTokens returns metadata without token values or hashes.
func (s *Store) ListTokens() ([]TokenRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, name, scopes, roles, expires_at, revoked_at, created_at
		FROM api_tokens ORDER BY created_at DESC, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []TokenRecord
	for rows.Next() {
		var record TokenRecord
		var scopesJSON, rolesJSON string
		var expiresAt, revokedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&record.ID, &record.Name, &scopesJSON, &rolesJSON, &expiresAt, &revokedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if err := decodeTokenRecord(&record, scopesJSON, rolesJSON, expiresAt, revokedAt, createdAt); err != nil {
			return nil, err
		}
		record.TokenHash = ""
		record.RevokedAt = parseOptionalTime(revokedAt)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tokens: %w", err)
	}
	return records, nil
}

// RevokeToken marks a token unusable while retaining its audit metadata.
func (s *Store) RevokeToken(id string) error {
	result, err := s.db.Exec(`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM api_tokens WHERE id = ?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrTokenNotFound
		}
	}
	return nil
}

func decodeTokenRecord(record *TokenRecord, scopesJSON, rolesJSON string, expiresAt, revokedAt sql.NullString, createdAt string) error {
	if err := json.Unmarshal([]byte(scopesJSON), &record.Scopes); err != nil {
		return fmt.Errorf("decode token scopes: %w", err)
	}
	if err := json.Unmarshal([]byte(rolesJSON), &record.Roles); err != nil {
		return fmt.Errorf("decode token roles: %w", err)
	}
	record.ExpiresAt = parseOptionalTime(expiresAt)
	record.RevokedAt = parseOptionalTime(revokedAt)
	if createdAt != "" {
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return fmt.Errorf("parse token creation time: %w", err)
		}
		record.CreatedAt = created
	}
	return nil
}

func parseOptionalTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func hashToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}

func normalizeScopes(scopes []string) ([]string, error) {
	result := append([]string(nil), scopes...)
	seen := make(map[string]struct{}, len(result))
	for _, scope := range result {
		if scope != scopeMCP && scope != "metrics" && scope != "logs" {
			return nil, fmt.Errorf("invalid token scope %q", scope)
		}
		if _, exists := seen[scope]; exists {
			return nil, fmt.Errorf("duplicate token scope %q", scope)
		}
		seen[scope] = struct{}{}
	}
	sort.Strings(result)
	return result, nil
}
