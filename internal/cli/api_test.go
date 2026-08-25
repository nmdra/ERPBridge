package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nmdra/ERPBridge/internal/idp"
	"github.com/nmdra/ERPBridge/internal/security"
	"github.com/stretchr/testify/require"
)

const (
	testAPIName        = "test-api"
	testURL            = "http://test"
	testActiveStatus   = "active"
	testBearerAuthType = "bearer"
)

func TestAPIRegistrationResponse_RenderTable(t *testing.T) {
	resp := &APIRegistrationResponse{
		API: idp.API{
			Name:   testAPIName,
			ID:     "123",
			Module: "finance",
			Method: http.MethodGet,
			URL:    testURL,
			Status: testActiveStatus,
		},
	}
	var buf bytes.Buffer
	err := resp.RenderTable(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte(testAPIName)) {
		t.Errorf("expected output to contain 'test-api'")
	}
}

func TestAPIListResponse_RenderTable(t *testing.T) {
	resp := &APIListResponse{
		Items: []idp.API{
			{ID: "1", Name: "api1", Module: "hr", Method: http.MethodPost, Status: testActiveStatus},
		},
	}
	var buf bytes.Buffer
	err := resp.RenderTable(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("api1")) {
		t.Errorf("expected output to contain 'api1'")
	}
}

func TestAPITestResponse_RenderTable(t *testing.T) {
	resp := &APITestResponse{
		API: idp.API{
			Name:       testAPIName,
			Method:     http.MethodGet,
			URL:        testURL,
			AuthType:   "api-key",
			AuthHeader: "X-Api-Key",
		},
		Status:    "200 OK",
		Latency:   time.Millisecond,
		IsSuccess: true,
	}
	var buf bytes.Buffer
	err := resp.RenderTable(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte(testAPIName)) {
		t.Errorf("expected output to contain 'test-api'")
	}

	resp.IsSuccess = false
	buf.Reset()
	require.NoError(t, resp.RenderTable(&buf))
	out = buf.String()
	if !bytes.Contains([]byte(out), []byte("failed")) {
		t.Errorf("expected output to contain 'failed'")
	}
}

func TestApiListCmd(t *testing.T) {
	setupTest()
	var buf bytes.Buffer
	formatter.Out = &buf

	err := apiListCmd.RunE(apiListCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApiTestCmd(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	api := &idp.API{
		Name:   "testapi",
		Method: http.MethodGet,
		URL:    ts.URL,
	}
	require.NoError(t, reg.Register(api))

	var buf bytes.Buffer
	formatter.Out = &buf
	apiTestCmd.SetContext(context.Background())
	err = apiTestCmd.RunE(apiTestCmd, []string{"testapi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPITestResolvesCredentialReference(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	// #nosec G101 -- test-only sentinel for the resolved environment credential.
	const credential = "api-test-secret"
	t.Setenv("ERP_API_TEST_KEY", credential)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+credential, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()
	t.Setenv(security.InsecureAuthAllowedHostsEnv, strings.TrimPrefix(ts.URL, "http://"))

	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	require.NoError(t, reg.Register(&idp.API{
		Name:          "secured-testapi",
		Method:        http.MethodGet,
		URL:           ts.URL,
		AuthType:      testBearerAuthType,
		CredentialRef: "ERP_API_TEST_KEY", // #nosec G101 -- environment-variable reference, not a secret.
	}))

	apiTestCmd.SetContext(context.Background())
	err = apiTestCmd.RunE(apiTestCmd, []string{"secured-testapi"})
	require.NoError(t, err)
}

func TestAPITestBlocksLegacyRegistry(t *testing.T) {
	setupTest()
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryDir := home + string(os.PathSeparator) + ".bridgectl"
	require.NoError(t, os.MkdirAll(registryDir, 0700))
	// #nosec G101 -- test-only sentinel proving legacy values are not exposed.
	const legacySecret = "legacy-api-test-secret"
	registry := `{"apis":{"legacy":{"name":"legacy","url":"https://example.invalid","authKey":"` + legacySecret + `"}}}`
	require.NoError(t, os.WriteFile(registryDir+string(os.PathSeparator)+"registry.json", []byte(registry), 0600))

	err := apiTestCmd.RunE(apiTestCmd, []string{"legacy"})
	require.ErrorIs(t, err, idp.ErrLegacyCredentials)
	require.NotContains(t, err.Error(), legacySecret)
}

func TestApiRegisterCmd(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	formatter.Out = &buf
	require.Nil(t, apiRegisterCmd.Flags().Lookup("auth-key"))
	require.NotNil(t, apiRegisterCmd.Flags().Lookup("credential-ref"))
	require.NoError(t, apiRegisterCmd.Flags().Set("credential-ref", ""))

	require.NoError(t, apiRegisterCmd.Flags().Set("name", "newapi"))
	require.NoError(t, apiRegisterCmd.Flags().Set("url", "http://test"))
	require.NoError(t, apiRegisterCmd.Flags().Set("module", "hr"))
	require.NoError(t, apiRegisterCmd.Flags().Set("description", "test desc"))

	err := apiRegisterCmd.RunE(apiRegisterCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	api, ok := reg.Get("newapi")
	require.True(t, ok)
	require.Equal(t, "", api.CredentialRef)
}

func TestAPITestRequiresCredentialReference(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	require.NoError(t, reg.Register(&idp.API{Name: "secured", URL: "https://example.invalid", Method: http.MethodGet, AuthType: testBearerAuthType}))
	err = apiTestCmd.RunE(apiTestCmd, []string{"secured"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "credential reference")
}

func TestAPIRegisterUsesCredentialReference(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, apiRegisterCmd.Flags().Set("name", "refapi"))
	require.NoError(t, apiRegisterCmd.Flags().Set("url", "https://example.invalid"))
	require.NoError(t, apiRegisterCmd.Flags().Set("module", "hr"))
	require.NoError(t, apiRegisterCmd.Flags().Set("description", "test desc"))
	require.NoError(t, apiRegisterCmd.Flags().Set("credential-ref", "ERP_CUSTOM_KEY"))
	require.NoError(t, apiRegisterCmd.RunE(apiRegisterCmd, nil))
	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	api, ok := reg.Get("refapi")
	require.True(t, ok)
	require.Equal(t, "ERP_CUSTOM_KEY", api.CredentialRef)
}

func TestAPISetCredentialRefCmd(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	reg, err := idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	require.NoError(t, reg.Register(&idp.API{Name: "setref", AuthType: testBearerAuthType}))
	require.NoError(t, apiSetCredentialRefCmd.Flags().Set("credential-ref", "ERP_SETREF_KEY"))
	require.NoError(t, apiSetCredentialRefCmd.RunE(apiSetCredentialRefCmd, []string{"setref"}))
	reg, err = idp.NewRegistry("", RootLog)
	require.NoError(t, err)
	api, ok := reg.Get("setref")
	require.True(t, ok)
	require.Equal(t, "ERP_SETREF_KEY", api.CredentialRef)
}

func TestAPIScrubCredentialsRequiresConfirmation(t *testing.T) {
	setupTest()
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, apiScrubCredentialsCmd.Flags().Set("yes", "false"))
	err := apiScrubCredentialsCmd.RunE(apiScrubCredentialsCmd, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--yes")
}

func TestAPIScrubCredentialsDoesNotPrintLegacySecret(t *testing.T) {
	setupTest()
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryDir := home + string(os.PathSeparator) + ".bridgectl"
	require.NoError(t, os.MkdirAll(registryDir, 0700))
	// #nosec G101 -- test-only sentinel proving scrub output is not secret-bearing.
	const legacySecret = "legacy-scrub-secret"
	registryPath := registryDir + string(os.PathSeparator) + "registry.json"
	registry := `{"apis":{"legacy":{"name":"legacy","url":"https://example.invalid","authToken":"` + legacySecret + `","module":"finance"}}}`
	require.NoError(t, os.WriteFile(registryPath, []byte(registry), 0600))

	var output bytes.Buffer
	apiScrubCredentialsCmd.SetOut(&output)
	apiScrubCredentialsCmd.SetErr(&output)
	require.NoError(t, apiScrubCredentialsCmd.Flags().Set("yes", "true"))
	require.NoError(t, apiScrubCredentialsCmd.RunE(apiScrubCredentialsCmd, nil))
	require.NotContains(t, output.String(), legacySecret)
	data, err := os.ReadFile(registryPath) // #nosec G304 -- test path is under t.TempDir.
	require.NoError(t, err)
	require.NotContains(t, string(data), legacySecret)
}
