package climcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestEndpointAliasesRejectedBeforeStaging(t *testing.T) {
	const unsafeEndpoint = "https://erp.internal/orders?password=review-canary"
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"spec alias", func(document map[string]any) {
			spec := document["spec"].(map[string]any)
			setReviewEndpoint(spec, document["kind"].(string), unsafeEndpoint)
			document["Spec"] = spec
			delete(document, "spec")
		}},
		{"mixed case spec alias", func(document map[string]any) {
			spec := document["spec"].(map[string]any)
			setReviewEndpoint(spec, document["kind"].(string), unsafeEndpoint)
			document["sPeC"] = spec
			delete(document, "spec")
		}},
		{"endpoint alias", func(document map[string]any) {
			endpointObject := reviewEndpointObject(document)
			delete(endpointObject, "endpoint")
			endpointObject["Endpoint"] = unsafeEndpoint
		}},
		{"conflicting endpoint aliases", func(document map[string]any) {
			// The canonical safe value must not hide a credential-bearing alias
			// that would otherwise reach the staging file.
			reviewEndpointObject(document)["Endpoint"] = unsafeEndpoint
		}},
		{"conflicting spec aliases", func(document map[string]any) {
			spec := map[string]any{}
			if document["kind"] == kindTool {
				spec["execution"] = map[string]any{}
			}
			setReviewEndpoint(spec, document["kind"].(string), unsafeEndpoint)
			document["Spec"] = spec
		}},
	}
	for _, kind := range []string{kindTool, kindPlugin} {
		for _, operation := range []string{"validate", "apply"} {
			tool := "bridgectl_tool_" + operation
			if kind == kindPlugin {
				tool = "bridgectl_plugin_" + operation
			}
			for _, testCase := range cases {
				t.Run(tool+"/"+testCase.name, func(t *testing.T) {
					manifest := validToolManifest()
					if kind == kindPlugin {
						manifest = validPluginManifest()
					}
					testCase.mutate(manifest)
					assertReviewManifestRejectedBeforeStaging(t, tool, manifest)
				})
			}
		}
	}
	for _, operation := range []string{"validate", "apply"} {
		for _, alias := range []string{"Execution", "eXeCuTiOn"} {
			t.Run("tool_"+operation+"/"+alias, func(t *testing.T) {
				manifest := validToolManifest()
				spec := manifest["spec"].(map[string]any)
				spec[alias] = map[string]any{"endpoint": unsafeEndpoint}
				delete(spec, "execution")
				assertReviewManifestRejectedBeforeStaging(t, "bridgectl_tool_"+operation, manifest)
			})
		}
	}
}

func reviewEndpointObject(document map[string]any) map[string]any {
	spec := document["spec"].(map[string]any)
	if document["kind"] == kindTool {
		return spec["execution"].(map[string]any)
	}
	return spec
}

func setReviewEndpoint(spec map[string]any, kind, endpoint string) {
	if kind == kindTool {
		spec["execution"].(map[string]any)["endpoint"] = endpoint
		return
	}
	spec["endpoint"] = endpoint
}

func assertReviewManifestRejectedBeforeStaging(t *testing.T, tool string, manifest map[string]any) {
	t.Helper()
	stages := 0
	ic := invocationContext{
		operation: tool, contextName: "test",
		stage: func(string, []byte) (string, error) {
			stages++
			return "unused-manifest.json", nil
		},
	}
	args := map[string]any{"manifest": manifest}
	if strings.HasSuffix(tool, "_apply") {
		name, version := manifestIdentity(manifest)
		args["confirmContext"] = "test"
		args["confirmTarget"] = targetOf(name, version)
	}
	_, failure := descriptorNamed(t, tool).invoke(ic, args)
	if stages != 0 {
		t.Errorf("staged %d manifest(s) before rejecting unsafe input", stages)
	}
	if failure == nil || failure.Error == nil || failure.Error.Code != CodeInvalidArguments {
		t.Fatalf("failure = %+v, want invalid_arguments", failure)
	}
	assertNoSecret(t, failure, "review-canary")
}

func TestManifestSharedSensitiveKeysRejectedBeforeStaging(t *testing.T) {
	for _, key := range []string{
		"client_secret", "access_token", "refresh_token", "private_key",
		"CLIENT_SECRET", " client_secret ", "upstream_access_token", "erp_password",
		"passwd", "apiKey", "authKey", "authToken", "credentialValue", "clientSecret", "privateKey", "bearer",
	} {
		for _, operation := range []string{"validate", "apply"} {
			t.Run(operation+"/"+key, func(t *testing.T) {
				manifest := validBindingManifest()
				manifest["spec"].(map[string]any)["config"] = map[string]any{
					"nested": []any{map[string]any{key: "review-canary"}},
				}
				assertReviewManifestRejectedBeforeStaging(t, "bridgectl_binding_"+operation, manifest)
			})
		}
	}
}

func TestSharedSensitiveSchemaDeclarationsRemainValid(t *testing.T) {
	for _, key := range []string{"client_secret", "access_token", "private_key", "upstream_access_token"} {
		t.Run(key, func(t *testing.T) {
			manifest := validToolManifest()
			manifest["spec"].(map[string]any)["inputSchema"] = map[string]any{
				"type": "object", "properties": map[string]any{key: map[string]any{"type": "string"}},
			}
			ic := testInvocationContext(t)
			if _, failure := ic.validateManifest(map[string]any{"manifest": manifest}, kindTool); failure != nil {
				t.Fatalf("schema declaration rejected: %+v", failure.Error)
			}
			manifest["spec"].(map[string]any)["inputSchema"].(map[string]any)["properties"].(map[string]any)[key].(map[string]any)["default"] = "review-canary"
			assertReviewManifestRejectedBeforeStaging(t, "bridgectl_tool_validate", manifest)
		})
	}
}

func TestResourceInventorySerializesEmptyItems(t *testing.T) {
	cases := []struct {
		name    string
		project func([]byte) (any, error)
		stdout  string
	}{
		{"tool", projectToolList, `{"tools":[]}`},
		{"plugin", projectPluginList, `{"plugins":[]}`},
		{"binding", projectBindingList, `{"bindings":[]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := testCase.project([]byte(testCase.stdout))
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewSuccess("bridgectl_"+testCase.name+"_get", "test", data).callToolResult()
			if err != nil {
				t.Fatal(err)
			}
			envelope := decodeEnvelope(t, result)
			fields := envelope.Data.(map[string]any)
			items, ok := fields["items"].([]any)
			if !ok || len(items) != 0 {
				t.Fatalf("items = %#v, want an empty array", fields["items"])
			}
			if _, present := fields["item"]; present {
				t.Fatal("inventory includes a named item")
			}
			if fields["redacted"] != true {
				t.Fatal("inventory is not marked redacted")
			}
		})
	}
}

func TestNamedResourceOmitsInventoryItems(t *testing.T) {
	for _, kind := range []string{kindTool, kindPlugin, kindBinding} {
		t.Run(kind, func(t *testing.T) {
			manifest := validToolManifest()
			project := projectToolItem
			switch kind {
			case kindPlugin:
				manifest, project = validPluginManifest(), projectPluginItem
			case kindBinding:
				manifest, project = validBindingManifest(), projectBindingItem
			}
			data, err := project([]byte(encodeFor(t, manifest)))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(encodeFor(t, data)), &fields); err != nil {
				t.Fatal(err)
			}
			if _, present := fields["item"]; !present {
				t.Fatal("named resource omits item")
			}
			if _, present := fields["items"]; present {
				t.Fatal("named resource includes inventory items")
			}
		})
	}
}
