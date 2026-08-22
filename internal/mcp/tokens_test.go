package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_TokenLifecycle(t *testing.T) {
	store, err := NewStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	record, raw, err := store.CreateToken(TokenCreateRequest{
		Name:   "integration-client",
		Scopes: []string{scopeMCP},
		Roles:  []string{testRoleZeta, testRoleAlpha},
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "erpbt_"))
	assert.Len(t, strings.TrimPrefix(raw, "erpbt_"), 64)
	assert.Equal(t, []string{testRoleAlpha, testRoleZeta}, record.Roles)
	assert.Equal(t, []string{scopeMCP}, record.Scopes)
	assert.NotContains(t, record.TokenHash, raw)

	var storedHash string
	require.NoError(t, store.db.QueryRow("SELECT token_hash FROM api_tokens WHERE id = ?", record.ID).Scan(&storedHash))
	assert.NotEqual(t, raw, storedHash)
	assert.NotEmpty(t, storedHash)

	found, err := store.LookupToken(raw)
	require.NoError(t, err)
	assert.Equal(t, record.ID, found.ID)
	assert.Equal(t, record.Roles, found.Roles)
	assert.Empty(t, found.TokenHash)

	listed, err := store.ListTokens()
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Empty(t, listed[0].TokenHash)

	require.NoError(t, store.RevokeToken(record.ID))
	_, err = store.LookupToken(raw)
	assert.ErrorIs(t, err, ErrTokenRevoked)
}

func TestStore_CreateTokenRejectsInvalidRoles(t *testing.T) {
	store, err := NewStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	for _, roles := range [][]string{{"Admin"}, {"a a"}, {"duplicate", "duplicate"}, {""}} {
		_, _, err := store.CreateToken(TokenCreateRequest{Name: "invalid", Roles: roles})
		assert.Error(t, err, "roles %v should be rejected", roles)
	}
}

func TestStore_LookupTokenRejectsExpiredToken(t *testing.T) {
	store, err := NewStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	_, raw, err := store.CreateToken(TokenCreateRequest{
		Name:      "expired-client",
		ExpiresAt: ptrTime(time.Now().Add(time.Minute)),
	})
	require.NoError(t, err)
	_, err = store.db.Exec("UPDATE api_tokens SET expires_at = ?", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano))
	require.NoError(t, err)

	_, err = store.LookupToken(raw)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestCallerIdentityContextCopiesRoles(t *testing.T) {
	original := CallerIdentity{PrincipalID: "token-1", Roles: []string{adminPrincipal}, IsAdmin: true}
	ctx := WithCallerIdentity(context.Background(), original)
	original.Roles[0] = "changed"

	identity, ok := CallerIdentityFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, adminPrincipal, identity.Roles[0])

	identity.Roles[0] = "mutated"
	second, ok := CallerIdentityFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, adminPrincipal, second.Roles[0])
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
