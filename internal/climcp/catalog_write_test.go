package climcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func validPluginManifest() map[string]any {
	return map[string]any{
		"apiVersion": "erpbridge.io/v1",
		"kind":       kindPlugin,
		"metadata":   map[string]any{"name": "redact", "version": "2.0.0", "type": "api"},
		"spec": map[string]any{
			"endpoint":            "https://plugins.internal/v1/process",
			"timeoutMilliseconds": 5000,
			"auth":                map[string]any{"type": "bearer", "credentialRef": "PLUGIN_REDACT_TOKEN"}, // #nosec G101 -- logical reference
		},
	}
}

func validBindingManifest() map[string]any {
	return map[string]any{
		"apiVersion": "erpbridge.io/v1",
		"kind":       kindBinding,
		"metadata":   map[string]any{"name": "orders-redaction"},
		"spec": map[string]any{
			"pluginRef":     map[string]any{"name": "redact", "version": "2.0.0"},
			"toolRef":       map[string]any{"name": "list_items", "version": "1.0.0"},
			"phase":         "after_response",
			"priority":      10,
			"failurePolicy": "fail",
		},
	}
}

// validWriteArgs returns accepted arguments for one write tool with matching
// confirmations.
func validWriteArgs(t *testing.T, name string) map[string]any {
	t.Helper()
	switch name {
	case "bridgectl_api_register":
		return map[string]any{
			"name": "erp", "url": "https://erp.internal/api/items", "module": "erp",
			"description": "ERP items", "confirmContext": "test", "confirmTarget": "erp",
		}
	case "bridgectl_api_set_credential_ref":
		return map[string]any{"name": "erp", "credentialRef": "ERP_PRIMARY_KEY", "confirmContext": "test", "confirmTarget": "erp"} // #nosec G101 -- logical reference
	case "bridgectl_api_test":
		return map[string]any{"name": "erp", "confirmContext": "test", "confirmTarget": "erp"}
	case "bridgectl_tool_apply":
		return map[string]any{"manifest": validToolManifest(), "confirmContext": "test", "confirmTarget": "list_items@1.0.0"}
	case "bridgectl_tool_delete":
		return map[string]any{"name": "list_items", "version": "1.0.0", "confirmContext": "test", "confirmTarget": "list_items@1.0.0"}
	case "bridgectl_plugin_apply":
		return map[string]any{"manifest": validPluginManifest(), "confirmContext": "test", "confirmTarget": "redact@2.0.0"}
	case "bridgectl_plugin_delete":
		return map[string]any{"name": "redact", "version": "2.0.0", "confirmContext": "test", "confirmTarget": "redact@2.0.0"}
	case "bridgectl_binding_apply":
		return map[string]any{"manifest": validBindingManifest(), "confirmContext": "test", "confirmTarget": "orders-redaction"}
	case "bridgectl_binding_delete":
		return map[string]any{"name": "orders-redaction", "confirmContext": "test", "confirmTarget": "orders-redaction"}
	case "bridgectl_token_revoke":
		return map[string]any{"id": "t1", "confirmContext": "test", "confirmTarget": "t1"}
	case "bridgectl_cache_flush":
		return map[string]any{"selector": "all", "confirmContext": "test", "confirmTarget": "all"}
	case "bridgectl_context_set":
		return map[string]any{"name": "staging", "confirmContext": "test", "confirmTarget": "staging"}
	}
	t.Fatalf("no valid arguments are defined for %s", name)
	return nil
}

// sessionWithGroups builds a session that can call the given write groups, with
// a child executable that does not exist so reaching execution is observable.
func sessionWithGroups(t *testing.T, groups ...WriteGroup) *Server {
	t.Helper()
	stager, err := NewStager()
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	t.Cleanup(func() { _ = stager.Cleanup() })
	session, err := NewServer(Options{
		Snapshot: testSnapshot(),
		Version:  "test-version",
		Groups:   groups,
		Stager:   stager,
		Runner:   &Runner{Executable: filepath.Join(t.TempDir(), "no-such-binary")},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return session
}

func TestWriteToolsAreHiddenWithoutTheirGroup(t *testing.T) {
	session := sessionWithGroups(t)
	advertised := session.ToolNames()
	if got, want := sortedCopy(advertised), defaultToolNames(); !equalStrings(got, want) {
		t.Fatalf("advertised = %v, want the read-only default set", got)
	}
	for name := range writeToolNames() {
		for _, tool := range advertised {
			if tool == name {
				t.Fatalf("%s is advertised without its startup group", name)
			}
		}
	}
}

func TestEachGroupAdvertisesExactlyItsTools(t *testing.T) {
	planned := writeToolNames()
	for _, group := range WriteGroups() {
		t.Run(string(group), func(t *testing.T) {
			session := sessionWithGroups(t, group)
			expected := append([]string(nil), defaultToolNames()...)
			for name, required := range planned {
				if required == group {
					expected = append(expected, name)
				}
			}
			got := session.ToolNames()
			if !equalStrings(sortedCopy(got), sortedCopy(expected)) {
				t.Fatalf("advertised = %v, want %v", sortedCopy(got), sortedCopy(expected))
			}
		})
	}
}

func TestForceRegistrationAlsoRequiresTheDestructiveGroup(t *testing.T) {
	registryOnly := sessionWithGroups(t, GroupRegistry)
	args := validWriteArgs(t, "bridgectl_api_register")
	args["force"] = true

	result := dispatchTool(t, registryOnly, "bridgectl_api_register", args)
	if result.Success {
		t.Fatal("force registration succeeded with only the registry group")
	}
	if result.Error.Code != CodePermissionDenied {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodePermissionDenied)
	}

	both := sessionWithGroups(t, GroupRegistry, GroupDestructive)
	result = dispatchTool(t, both, "bridgectl_api_register", args)
	if result.Success {
		t.Fatal("the test child executable should not exist")
	}
	if result.Error.Code == CodePermissionDenied {
		t.Fatalf("both groups were rejected: %s", result.Error.Message)
	}
}

func TestEveryWriteToolRequiresMatchingConfirmation(t *testing.T) {
	planned := writeToolNames()
	for name, group := range planned {
		t.Run(name, func(t *testing.T) {
			session := sessionWithGroups(t, group, GroupRegistry, GroupResources, GroupDestructive, GroupProbe, GroupContext)
			descriptor := descriptorNamed(t, name)

			// Wrong context.
			args := validWriteArgs(t, name)
			args["confirmContext"] = "somewhere-else"
			result := decodeEnvelope(t, mustDispatch(t, session, descriptor, args))
			if result.Success || result.Error.Code != CodeConfirmationMismatch {
				t.Fatalf("wrong confirmContext: %+v", result)
			}

			// Wrong target.
			args = validWriteArgs(t, name)
			args["confirmTarget"] = "not-the-target"
			result = decodeEnvelope(t, mustDispatch(t, session, descriptor, args))
			if result.Success || result.Error.Code != CodeConfirmationMismatch {
				t.Fatalf("wrong confirmTarget: %+v", result)
			}

			// Missing confirmation.
			args = validWriteArgs(t, name)
			delete(args, "confirmTarget")
			result = decodeEnvelope(t, mustDispatch(t, session, descriptor, args))
			if result.Success || result.Error.Code != CodeInvalidArguments {
				t.Fatalf("missing confirmTarget: %+v", result)
			}

			// Unknown argument must be rejected before confirmation is accepted.
			args = validWriteArgs(t, name)
			args["unexpected"] = true
			result = decodeEnvelope(t, mustDispatch(t, session, descriptor, args))
			if result.Success || result.Error.Code != CodeInvalidArguments {
				t.Fatalf("unknown argument: %+v", result)
			}
		})
	}
}

func TestConfirmationMismatchSkipsExecution(t *testing.T) {
	session := sessionWithGroups(t, GroupRegistry, GroupResources, GroupDestructive, GroupProbe, GroupContext)
	for name, group := range writeToolNames() {
		t.Run(name, func(t *testing.T) {
			args := validWriteArgs(t, name)
			args["confirmTarget"] = "not-the-target"
			result := dispatchTool(t, session, name, args)
			if result.Success {
				t.Fatal("a mismatched confirmation succeeded")
			}
			// The child executable does not exist, so any execution attempt would
			// surface as an internal error instead of a policy error.
			if result.Error.Code == CodeInternal || result.Error.Code == CodeOutcomeUnknown {
				t.Fatalf("%s reached execution despite a mismatch: %+v", name, result)
			}
			_ = group
		})
	}
}

func TestWriteArgumentVectors(t *testing.T) {
	ic := testInvocationContext(t)
	cases := []struct {
		name string
		args map[string]any
		want []string
	}{
		{"bridgectl_api_set_credential_ref",
			map[string]any{"name": "erp", "credentialRef": "ERP_PRIMARY_KEY", "confirmContext": "test", "confirmTarget": "erp"}, // #nosec G101 -- logical reference
			[]string{flagOutput, formatJSON, commandAPI, "set-credential-ref", "--credential-ref=ERP_PRIMARY_KEY", separator, "erp"}},
		{"bridgectl_api_test",
			map[string]any{"name": "erp", "confirmContext": "test", "confirmTarget": "erp"},
			[]string{flagOutput, formatJSON, commandAPI, "test", separator, "erp"}},
		{"bridgectl_tool_delete",
			map[string]any{"name": "list_items", "version": "1.0.0", "confirmContext": "test", "confirmTarget": "list_items@1.0.0"},
			[]string{flagOutput, formatJSON, commandTool, "delete", separator, "list_items", "1.0.0"}},
		{"bridgectl_tool_delete",
			map[string]any{"name": "list_items", "version": "1.0.0", "hard": true, "confirmContext": "test", "confirmTarget": "list_items@1.0.0"},
			[]string{flagOutput, formatJSON, commandTool, "delete", "--hard", "--yes", separator, "list_items", "1.0.0"}},
		{"bridgectl_plugin_delete",
			map[string]any{"name": "redact", "version": "2.0.0", "confirmContext": "test", "confirmTarget": "redact@2.0.0"},
			[]string{flagOutput, formatJSON, commandPlugin, "delete", separator, "redact@2.0.0"}},
		{"bridgectl_binding_delete",
			map[string]any{"name": "orders-redaction", "confirmContext": "test", "confirmTarget": "orders-redaction"},
			[]string{flagOutput, formatJSON, commandPlugin, "binding", "delete", separator, "orders-redaction"}},
		{"bridgectl_token_revoke",
			map[string]any{"id": "t1", "confirmContext": "test", "confirmTarget": "t1"},
			[]string{flagOutput, formatJSON, "token", "revoke", separator, "t1"}},
		{"bridgectl_cache_flush",
			map[string]any{"selector": "tool", "name": "list_items", "confirmContext": "test", "confirmTarget": "tool:list_items"},
			[]string{flagOutput, formatJSON, "cache", "flush", separator, "list_items"}},
		{"bridgectl_cache_flush",
			map[string]any{"selector": "module", "name": "erp", "confirmContext": "test", "confirmTarget": "module:erp"},
			[]string{flagOutput, formatJSON, "cache", "flush", "--module=erp"}},
		{"bridgectl_cache_flush",
			map[string]any{"selector": "all", "confirmContext": "test", "confirmTarget": "all"},
			[]string{flagOutput, formatJSON, "cache", "flush", "--all"}},
		{"bridgectl_context_set",
			map[string]any{"name": "staging", "confirmContext": "test", "confirmTarget": "staging"},
			[]string{flagOutput, formatJSON, "context", "set", separator, "staging"}},
	}
	for _, testCase := range cases {
		descriptor := descriptorNamed(t, testCase.name)
		inv, failure := descriptor.invoke(ic, testCase.args)
		if failure != nil {
			t.Fatalf("%s%v rejected valid arguments: %s", testCase.name, testCase.args, failure.Error.Message)
		}
		if !equalStrings(inv.argv, testCase.want) {
			t.Fatalf("%s argv = %v, want %v", testCase.name, inv.argv, testCase.want)
		}
	}
}

func TestRegistrationArgumentValidation(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_api_register")
	base := func() map[string]any {
		return map[string]any{
			"name": "erp", "url": "https://erp.internal/api/items", "module": "erp",
			"description": "ERP items", "confirmContext": "test", "confirmTarget": "erp",
		}
	}
	rejects := []struct {
		label  string
		mutate func(map[string]any)
	}{
		{"userinfo url", func(args map[string]any) { args["url"] = "https://user:pass@erp.internal/api" }}, // #nosec G101 -- canary userinfo
		{"query credentials", func(args map[string]any) { args["url"] = "https://erp.internal/api?token=canary" }},
		{"fragment url", func(args map[string]any) { args["url"] = "https://erp.internal/api#x" }},
		{"relative url", func(args map[string]any) { args["url"] = "/api/items" }},
		{"non-http url", func(args map[string]any) { args["url"] = "file:///etc/passwd" }},
		{"unknown method", func(args map[string]any) { args["method"] = "TRACE" }},
		{"unknown auth type", func(args map[string]any) { args["authType"] = "oauth" }},
		{"header value instead of name", func(args map[string]any) { args["authHeader"] = "Bearer canary-secret" }}, // #nosec G101 -- canary value
		{"option injection in name", func(args map[string]any) { args["name"] = "--force" }},
	}
	for _, testCase := range rejects {
		t.Run(testCase.label, func(t *testing.T) {
			args := base()
			testCase.mutate(args)
			if failure := registrationFailure(descriptor, ic, args); failure == nil {
				t.Fatal("an unsafe registration was accepted")
			} else if failure.Error.Code != CodeInvalidArguments {
				t.Fatalf("code = %q, want %q", failure.Error.Code, CodeInvalidArguments)
			}
		})
	}
}

// registrationFailure runs the registration builder, adjusting the confirmation
// when the test changed the target identity.
func registrationFailure(d descriptor, ic invocationContext, args map[string]any) *Result {
	if name, ok := args["name"].(string); ok {
		args["confirmTarget"] = name
	}
	_, failure := d.invoke(ic, args)
	return failure
}

func TestCacheFlushSelectorRules(t *testing.T) {
	ic := testInvocationContext(t)
	descriptor := descriptorNamed(t, "bridgectl_cache_flush")
	cases := []struct {
		label string
		args  map[string]any
		code  string
	}{
		{"all with a name", map[string]any{"selector": "all", "name": "erp", "confirmContext": "test", "confirmTarget": "all"}, CodeInvalidArguments},
		{"tool without a name", map[string]any{"selector": "tool", "confirmContext": "test", "confirmTarget": "tool:"}, CodeInvalidArguments},
		{"unknown selector", map[string]any{"selector": "everything", "confirmContext": "test", "confirmTarget": "everything"}, CodeInvalidArguments},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			_, failure := descriptor.invoke(ic, testCase.args)
			if failure == nil || failure.Error.Code != testCase.code {
				t.Fatalf("failure = %+v, want %q", failure, testCase.code)
			}
		})
	}
}

func TestWriteFailuresReportUnknownOutcome(t *testing.T) {
	session := sessionWithGroups(t, GroupResources)
	descriptor := descriptorNamed(t, "bridgectl_tool_apply")

	cases := []struct {
		label string
		err   error
		out   Outcome
		want  string
	}{
		{"timeout", ErrTimeout, Outcome{}, CodeOutcomeUnknown},
		{"cancelled", ErrCancelled, Outcome{}, CodeOutcomeUnknown},
		{"output limit", ErrOutputLimit, Outcome{}, CodeOutcomeUnknown},
		{"busy", ErrBusy, Outcome{}, CodeBusy},
		{"child failed", nil, Outcome{ExitCode: 1}, CodeOutcomeUnknown},
		{"context changed", nil, Outcome{ExitCode: 7, Stderr: []byte(`{"error":"MCP_CONTEXT_CHANGED"}`)}, CodeContextChanged},
		{"bad arguments", nil, Outcome{ExitCode: 2}, CodeInvalidArguments},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			result := session.classifyWriteFailure(descriptor, testCase.out, testCase.err)
			if result.Error.Code != testCase.want {
				t.Fatalf("code = %q, want %q", result.Error.Code, testCase.want)
			}
			if testCase.want == CodeOutcomeUnknown {
				if result.Error.Cause == "" {
					t.Fatal("an unknown outcome carried no cause")
				}
				if !strings.Contains(result.Error.Suggestion, "Read the target resource") {
					t.Fatalf("suggestion = %q, want read-back guidance", result.Error.Suggestion)
				}
			}
		})
	}
}

func TestWriteSuccessNeverClaimsVerification(t *testing.T) {
	session := sessionWithGroups(t, GroupContext)
	descriptor := descriptorNamed(t, "bridgectl_context_set")
	inv, failure := descriptor.invoke(testInvocationContext(t), validWriteArgs(t, "bridgectl_context_set"))
	if failure != nil {
		t.Fatalf("invoke: %s", failure.Error.Message)
	}
	result := session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: []byte("ignored")}, nil)
	if !result.Success {
		t.Fatalf("write did not succeed: %+v", result)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal write data: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode write data: %v", err)
	}
	if data["verified"] != false {
		t.Fatalf("write result claims verification: %v", data)
	}
	if data["activeContext"] != "test" {
		t.Fatalf("saved-default change reported a different active context: %v", data)
	}
	if data["savedDefault"] != "staging" {
		t.Fatalf("saved-default change did not report the requested name: %v", data)
	}

	// A resource apply reports command completion for its canonical target.
	applySession := sessionWithGroups(t, GroupResources)
	apply := descriptorNamed(t, "bridgectl_tool_apply")
	applyInvocation, applyFailure := apply.invoke(testInvocationContext(t), validWriteArgs(t, "bridgectl_tool_apply"))
	if applyFailure != nil {
		t.Fatalf("apply invoke: %s", applyFailure.Error.Message)
	}
	applied := applySession.finish(apply, applyInvocation, Outcome{ExitCode: 0, Stdout: []byte("applied")}, nil)
	if !applied.Success {
		t.Fatalf("apply did not succeed: %+v", applied)
	}
	applyData := decodeData[WriteData](t, applied.Data)
	if applyData.Effect != effectCommandCompleted || applyData.Verified || applyData.Target != "list_items@1.0.0" {
		t.Fatalf("apply data = %+v", applyData)
	}
}

func TestProbeFailureIsACompletedObservation(t *testing.T) {
	session := sessionWithGroups(t, GroupProbe)
	descriptor := descriptorNamed(t, "bridgectl_api_test")
	inv, failure := descriptor.invoke(testInvocationContext(t), validWriteArgs(t, "bridgectl_api_test"))
	if failure != nil {
		t.Fatalf("invoke: %s", failure.Error.Message)
	}

	// The CLI reports a failed probe with exit code 0 and isSuccess false. The
	// latency is a Go duration in nanoseconds.
	stdout := []byte(`{"api":"erp","status":"401 Unauthorized","code":401,"latency":250000000,"contentType":"application/json","isSuccess":false}`)
	result := session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: stdout}, nil)
	if result.Success {
		t.Fatal("a failed probe was reported as success")
	}
	if result.Error.Code != CodeProbeFailed {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeProbeFailed)
	}
	if result.Error.Details == nil {
		t.Fatal("probe failure carried no typed details")
	}
	if result.Error.Details.HTTPStatus != 401 || result.Error.Details.LatencyMs != 250 || result.Error.Details.ProbeSucceeded {
		t.Fatalf("probe details = %+v", result.Error.Details)
	}

	success := []byte(`{"api":"erp","status":"200 OK","code":200,"latency":1500000,"contentType":"application/json","isSuccess":true}`)
	result = session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: success}, nil)
	if !result.Success {
		t.Fatalf("a successful probe failed: %+v", result)
	}
	data := decodeData[ProbeData](t, result.Data)
	if data.HTTPStatus != 200 || data.LatencyMs != 1 || !data.ProbeSucceeded {
		t.Fatalf("probe data = %+v", data)
	}
}

func TestWriteWithUnusableOutputIsUnknown(t *testing.T) {
	session := sessionWithGroups(t, GroupDestructive)
	descriptor := descriptorNamed(t, "bridgectl_cache_flush")
	inv, failure := descriptor.invoke(testInvocationContext(t), validWriteArgs(t, "bridgectl_cache_flush"))
	if failure != nil {
		t.Fatalf("invoke: %s", failure.Error.Message)
	}
	result := session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: []byte("not json")}, nil)
	if result.Success {
		t.Fatal("unusable write output was reported as success")
	}
	if result.Error.Code != CodeOutcomeUnknown {
		t.Fatalf("code = %q, want %q", result.Error.Code, CodeOutcomeUnknown)
	}
}

func TestCacheFlushReportsCounters(t *testing.T) {
	session := sessionWithGroups(t, GroupDestructive)
	descriptor := descriptorNamed(t, "bridgectl_cache_flush")
	inv, failure := descriptor.invoke(testInvocationContext(t), map[string]any{
		"selector": "module", "name": "erp", "confirmContext": "test", "confirmTarget": "module:erp",
	})
	if failure != nil {
		t.Fatalf("invoke: %s", failure.Error.Message)
	}
	result := session.finish(descriptor, inv, Outcome{ExitCode: 0, Stdout: []byte(`{"deleted":12,"status":"ok"}`)}, nil)
	if !result.Success {
		t.Fatalf("flush failed: %+v", result)
	}
	data := decodeData[CacheFlushData](t, result.Data)
	if data.Deleted != 12 || data.Target != "module:erp" || data.Verified {
		t.Fatalf("flush data = %+v", data)
	}
}
