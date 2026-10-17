package climcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Fixed argument tokens. Every child invocation is built from these, so the
// argv is never assembled from caller-supplied text.
const (
	flagOutput      = "-o"
	formatJSON      = "json"
	flagFile        = "-f"
	separator       = "--"
	commandGet      = "get"
	commandList     = "list"
	commandTool     = "tool"
	commandPlugin   = "plugin"
	commandValidate = "validate"
)

// Bounded argument limits. Every caller-supplied string is checked against
// these before it reaches a fixed argument vector.
const (
	maxIdentifierBytes  = 128
	maxModuleBytes      = 64
	maxDescriptionBytes = 2048
	maxURIBytes         = 4096
	maxArgumentKeyBytes = 64
)

// Resource kinds accepted by the manifest tools.
const (
	kindTool    = "MCPTool"
	kindPlugin  = "Plugin"
	kindBinding = "PluginBinding"
)

// sideEffect describes what a descriptor does. It decides how a failure is
// classified, so a launched mutation is never reported as a clean failure.
type sideEffect int

const (
	// effectRead reads local or remote state without changing it.
	effectRead sideEffect = iota
	// effectLocal validates or generates locally with no remote call.
	effectLocal
)

// invocation is one validated, fixed child call. project converts bounded child
// stdout into safe result data.
type invocation struct {
	argv    []string
	project func(stdout []byte) (any, error)
}

// invocationContext carries the session facts an invocation builder needs.
type invocationContext struct {
	operation   string
	contextName string
	stage       func(name string, data []byte) (string, error)
}

// fail builds a complete failure envelope for this operation.
func (ic invocationContext) fail(code, message, suggestion string) *Result {
	return NewFailure(ic.operation, ic.contextName, code, message, suggestion)
}

// descriptor is one fixed management tool. Registration and dispatch share
// this record, so an advertised tool always has a matching fixed operation and
// permission requirement.
type descriptor struct {
	// name is the advertised MCP tool name.
	name string
	// requiredGroup is the startup write group needed to advertise and call
	// this tool. An empty group means inspection or local-only work.
	requiredGroup WriteGroup
	// effect is the side-effect class used when classifying a failure.
	effect sideEffect
	// localValidation marks a tool whose non-zero exit means the submitted
	// resource failed local validation rather than an operational failure.
	localValidation bool
	// namedLookup marks a tool that reads one named resource, so the CLI's
	// recognized "not found" outcome maps to a precise result.
	namedLookup bool
	// tool is the advertised schema.
	tool mcp.Tool
	// invoke validates arguments and builds the fixed child invocation.
	invoke func(ic invocationContext, args map[string]any) (invocation, *Result)
}

// registerCatalogue registers every permitted tool from the fixed descriptor
// table.
func registerCatalogue(s *Server) {
	for _, entry := range catalogue {
		if entry.requiredGroup != "" && !s.Allows(entry.requiredGroup) {
			continue
		}
		s.register(entry.tool, s.handler(entry))
	}
}

// handler adapts a descriptor to the SDK handler signature.
func (s *Server) handler(d descriptor) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, d, request.GetArguments())
	}
}

// dispatch runs the fixed pipeline: validate, stage, execute, project. It never
// returns raw child output to the caller.
func (s *Server) dispatch(ctx context.Context, d descriptor, args map[string]any) (*mcp.CallToolResult, error) {
	if d.requiredGroup != "" && !s.Allows(d.requiredGroup) {
		failure := NewFailure(d.name, s.snapshot.ContextName, CodePermissionDenied,
			"this tool was not approved at startup",
			"Restart the management MCP process with the required --allow-write group.")
		return failure.callToolResult()
	}

	var stagedPath string
	ic := invocationContext{
		operation:   d.name,
		contextName: s.snapshot.ContextName,
		stage: func(name string, data []byte) (string, error) {
			path, err := s.stager.Stage(name, data)
			if err == nil {
				stagedPath = path
			}
			return path, err
		},
	}

	inv, failure := d.invoke(ic, args)
	defer func() {
		if stagedPath != "" {
			_ = os.Remove(stagedPath)
		}
	}()
	if failure != nil {
		return failure.callToolResult()
	}

	outcome, err := s.runner.Run(ctx, CommandSpec{Argv: inv.argv})
	result := s.finish(d, inv, outcome, err)
	return result.callToolResult()
}

// finish converts one child run into an adapter envelope.
func (s *Server) finish(d descriptor, inv invocation, outcome Outcome, err error) *Result {
	if err != nil {
		return s.classifyRunFailure(d, outcome, err)
	}
	if outcome.ExitCode != 0 {
		return s.classifyExitFailure(d, outcome)
	}
	data, projectErr := inv.project(outcome.Stdout)
	if projectErr != nil {
		return NewFailure(d.name, s.snapshot.ContextName, CodeInvalidChildOutput,
			"the command completed but its output could not be interpreted",
			"Use the bridgectl CLI directly to inspect the raw result.")
	}
	return NewSuccess(d.name, s.snapshot.ContextName, data)
}

// classifyRunFailure maps a runner error to a safe envelope. Read and local
// tools cannot have committed a change, so their failures stay specific.
func (s *Server) classifyRunFailure(d descriptor, _ Outcome, err error) *Result {
	switch {
	case errors.Is(err, ErrBusy):
		return NewFailure(d.name, s.snapshot.ContextName, CodeBusy,
			"another management operation is already running",
			"Retry after the current operation finishes.")
	case errors.Is(err, ErrOutputLimit):
		return NewFailure(d.name, s.snapshot.ContextName, CodeOutputLimit,
			"the command produced more output than the adapter accepts",
			"Use the bridgectl CLI directly to inspect the full result.")
	case errors.Is(err, ErrTimeout):
		return NewFailure(d.name, s.snapshot.ContextName, CodeTimeout,
			"the command did not finish within the call deadline",
			"Check that the pinned endpoint is reachable, then retry.")
	case errors.Is(err, ErrCancelled):
		return NewFailure(d.name, s.snapshot.ContextName, CodeCancelled,
			"the operation was cancelled before it finished",
			"Retry when the caller is ready.")
	default:
		return NewFailure(d.name, s.snapshot.ContextName, CodeInternal,
			"the management command could not be started",
			"Run bridgectl directly to diagnose the local environment.")
	}
}

// classifyExitFailure maps a non-zero child exit to a safe envelope, using only
// recognized error codes.
func (s *Server) classifyExitFailure(d descriptor, outcome Outcome) *Result {
	if code, ok := childErrorCode(outcome.Stdout, outcome.Stderr); ok {
		if mapped, known := childFailureCodes[code]; known {
			return NewFailure(d.name, s.snapshot.ContextName, mapped.code, mapped.message, mapped.suggestion)
		}
	}
	if d.localValidation && outcome.ExitCode != 3 {
		return NewFailure(d.name, s.snapshot.ContextName, CodeInvalidArguments,
			"the submitted resource failed local validation",
			"Run bridgectl validate with the same file to see the exact reason.")
	}
	if d.namedLookup && outcome.ExitCode != 0 && strings.Contains(string(outcome.Stderr), notFoundPhrase) {
		return NewFailure(d.name, s.snapshot.ContextName, CodeNotFound,
			"the requested resource was not found",
			"Inspect the resource identity and retry.")
	}
	if mapped, known := exitCodeFailures[outcome.ExitCode]; known {
		return NewFailure(d.name, s.snapshot.ContextName, mapped.code, mapped.message, mapped.suggestion)
	}
	return NewFailure(d.name, s.snapshot.ContextName, CodeInternal,
		"the management command failed",
		"Run bridgectl directly to see the full diagnostic.")
}

// ---------------------------------------------------------------------------
// Argument validation
// ---------------------------------------------------------------------------

// rejectUnknown enforces the declared argument set. The SDK validates the
// schema at the transport, and this check repeats it at dispatch, so an
// uncompilable schema cannot widen the accepted input.
func (ic invocationContext) rejectUnknown(args map[string]any, allowed ...string) *Result {
	permitted := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		permitted[key] = true
	}
	for key := range args {
		if permitted[key] {
			continue
		}
		return ic.fail(CodeInvalidArguments,
			fmt.Sprintf("unknown argument %q", boundedKey(key)),
			"Send only the documented arguments for this tool.")
	}
	return nil
}

// boundedKey keeps an unexpected argument name short and printable before it is
// echoed in a message.
func boundedKey(key string) string {
	if len(key) > maxArgumentKeyBytes {
		key = key[:maxArgumentKeyBytes]
	}
	var builder strings.Builder
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// optionalString returns a validated optional string argument.
func (ic invocationContext) optionalString(args map[string]any, key string, limit int) (string, *Result) {
	raw, present := args[key]
	if !present || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", ic.fail(CodeInvalidArguments,
			fmt.Sprintf("%s must be a string", key),
			"Correct the argument and retry.")
	}
	if problem := identifierProblem(value, limit); problem != "" {
		return "", ic.fail(CodeInvalidArguments,
			fmt.Sprintf("%s %s", key, problem),
			"Correct the argument and retry.")
	}
	return value, nil
}

// requiredString returns a validated required string argument.
func (ic invocationContext) requiredString(args map[string]any, key string, limit int) (string, *Result) {
	value, failure := ic.optionalString(args, key, limit)
	if failure != nil {
		return "", failure
	}
	if value == "" {
		return "", ic.fail(CodeInvalidArguments,
			fmt.Sprintf("%s is required", key),
			"Provide the argument and retry.")
	}
	return value, nil
}

// identifierProblem reports why a value is not usable as a fixed argument, or
// an empty string when it is acceptable.
func identifierProblem(value string, limit int) string {
	if value == "" {
		return "must not be empty"
	}
	if len(value) > limit {
		return fmt.Sprintf("must be at most %d bytes", limit)
	}
	if strings.HasPrefix(value, "-") {
		return "must not start with '-'"
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "must not contain control characters"
		}
	}
	return ""
}

// versionedIdentifier composes the CLI's name@version form from validated parts.
func (ic invocationContext) versionedIdentifier(args map[string]any) (string, *Result) {
	name, failure := ic.optionalString(args, "name", maxIdentifierBytes)
	if failure != nil {
		return "", failure
	}
	version, failure := ic.optionalString(args, "version", maxIdentifierBytes)
	if failure != nil {
		return "", failure
	}
	if name == "" && version != "" {
		return "", ic.fail(CodeInvalidArguments,
			"version requires name",
			"Provide name, or omit both for a list request.")
	}
	if strings.Contains(name, "@") || strings.Contains(version, "@") {
		return "", ic.fail(CodeInvalidArguments,
			"name and version must not contain '@'",
			"Provide the name and version as separate arguments.")
	}
	if version == "" {
		return name, nil
	}
	return name + "@" + version, nil
}

// ---------------------------------------------------------------------------
// Manifest validation
// ---------------------------------------------------------------------------

// stagedManifest is a validated manifest that has been written to the private
// staging directory.
type stagedManifest struct {
	path    string
	kind    string
	name    string
	version string
}

// prepareManifest validates one inline resource manifest and stages it for the
// child. Validation happens before staging, so an invalid or credential-bearing
// document never reaches the filesystem.
func (ic invocationContext) prepareManifest(args map[string]any, expectedKind string) (stagedManifest, *Result) {
	raw, present := args["manifest"]
	if !present || raw == nil {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"manifest is required",
			"Provide one inline resource object.")
	}
	document, ok := raw.(map[string]any)
	if !ok {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"manifest must be a single JSON object",
			"Provide one inline object, not a path, URL, array, or list.")
	}

	kind, _ := document["kind"].(string)
	if kind != "" && kind != expectedKind {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			fmt.Sprintf("manifest kind must be %s", expectedKind),
			"Send the resource kind this tool accepts.")
	}
	name, version := manifestIdentity(document)
	if name == "" {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"manifest.metadata.name is required",
			"Provide the resource name inside the manifest.")
	}
	if problem := identifierProblem(name, maxIdentifierBytes); problem != "" {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"manifest.metadata.name "+problem,
			"Correct the manifest and retry.")
	}
	if scanCredentialValues(document, false) {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"the manifest contains a literal credential value",
			"Reference credentials by logical name only; never send credential values.")
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			"the manifest could not be encoded",
			"Correct the manifest and retry.")
	}
	if len(encoded) > MaxManifestBytes {
		return stagedManifest{}, ic.fail(CodeInvalidArguments,
			fmt.Sprintf("the manifest exceeds %d bytes", MaxManifestBytes),
			"Send a smaller resource definition.")
	}
	path, err := ic.stage("manifest.json", encoded)
	if err != nil {
		return stagedManifest{}, ic.fail(CodeInternal,
			"the manifest could not be staged for validation",
			"Check that the local temporary directory is writable.")
	}
	return stagedManifest{path: path, kind: expectedKind, name: name, version: version}, nil
}

// manifestIdentity reads the resource identity without decoding the whole
// document.
func manifestIdentity(document map[string]any) (name, version string) {
	metadata, ok := document["metadata"].(map[string]any)
	if !ok {
		return "", ""
	}
	name, _ = metadata["name"].(string)
	version, _ = metadata["version"].(string)
	return name, version
}

// notFoundPhrase is the CLI's recognized missing-resource wording. A reworded
// message degrades to a generic failure rather than a wrong not-found result.
const notFoundPhrase = " not found"

// credentialValueKeys are value-bearing credential keys. They are rejected
// outside a schema declaration, because a tool may legitimately declare an
// input property named token or password.
var credentialValueKeys = map[string]bool{
	"password": true, "passwd": true, "token": true, "secret": true,
	"apikey": true, "api_key": true, "authkey": true, "authtoken": true,
	"credentialvalue": true, "clientsecret": true, "privatekey": true,
	"bearer": true, "authorization": true,
}

// schemeKeys are subtrees whose property names are declarations, not values.
var schemeKeys = map[string]bool{"inputschema": true, "outputschema": true, "properties": true}

// scanCredentialValues reports the path of the first non-empty credential-like
// value found outside a schema declaration.
func scanCredentialValues(node any, insideSchema bool) bool {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			lower := strings.ToLower(key)
			if schemeKeys[lower] {
				if scanCredentialValues(value, true) {
					return true
				}
				continue
			}
			if !insideSchema && credentialValueKeys[lower] {
				if text, isString := value.(string); isString && text != "" {
					return true
				}
			}
			if scanCredentialValues(value, insideSchema) {
				return true
			}
		}
	case []any:
		for _, entry := range typed {
			if scanCredentialValues(entry, insideSchema) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Descriptor table
// ---------------------------------------------------------------------------

// catalogue is the complete fixed tool list. Adding an entry requires a plan
// amendment: no generic command, shell, path, or URL tool may be added here.
var catalogue = []descriptor{
	contextListDescriptor,
	apiListDescriptor,
	toolGetDescriptor,
	toolGenerateDescriptor,
	toolValidateDescriptor,
	pluginGetDescriptor,
	pluginValidateDescriptor,
	bindingGetDescriptor,
	bindingValidateDescriptor,
	tokenListDescriptor,
	cacheStatsDescriptor,
	logStatsDescriptor,
}

// noArgumentInvocation builds a read invocation that accepts no arguments.
func noArgumentInvocation(argv []string, project func([]byte) (any, error)) func(invocationContext, map[string]any) (invocation, *Result) {
	return func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args); failure != nil {
			return invocation{}, failure
		}
		return invocation{argv: argv, project: project}, nil
	}
}

var contextListDescriptor = descriptor{
	name:   "bridgectl_context_list",
	effect: effectRead,
	tool: mcp.NewTool("bridgectl_context_list",
		mcp.WithDescription("List saved bridgectl contexts. Returns names only, with the context pinned to this session marked."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	),
	invoke: noArgumentInvocation([]string{flagOutput, formatJSON, "context", commandList}, projectContextList),
}

var apiListDescriptor = descriptor{
	name:   "bridgectl_api_list",
	effect: effectRead,
	tool: mcp.NewTool("bridgectl_api_list",
		mcp.WithDescription("List the ERP APIs registered in the local registry for the pinned context. Credential values are never returned."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	),
	invoke: noArgumentInvocation([]string{flagOutput, formatJSON, "api", commandList}, projectAPIList),
}

var toolGetDescriptor = descriptor{
	name:        "bridgectl_tool_get",
	effect:      effectRead,
	namedLookup: true,
	tool: mcp.NewTool("bridgectl_tool_get",
		mcp.WithDescription("Read tool resources from the pinned deployment. Omit name to list; provide name (and optionally version) to read one. The result is a redacted inspection view."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("name", mcp.Description("Tool name to read. Omit to list every tool.")),
		mcp.WithString("version", mcp.Description("Exact version to read. Requires name.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "version"); failure != nil {
			return invocation{}, failure
		}
		identifier, failure := ic.versionedIdentifier(args)
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandTool, commandGet, flagOutput, formatJSON}
		if identifier == "" {
			return invocation{argv: argv, project: projectToolList}, nil
		}
		argv = append(argv, "--", identifier)
		return invocation{argv: argv, project: projectToolItem}, nil
	},
}

var toolGenerateDescriptor = descriptor{
	name:   "bridgectl_tool_generate",
	effect: effectLocal,
	tool: mcp.NewTool("bridgectl_tool_generate",
		mcp.WithDescription("Generate the tool manifest for an already-registered API. Returns the executable manifest so it can be applied. Remote OpenAPI fetch and file output are not available here."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("api", mcp.Required(), mcp.Description("Name of the registered API to generate from.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "api"); failure != nil {
			return invocation{}, failure
		}
		api, failure := ic.requiredString(args, "api", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		return invocation{
			argv:    []string{flagOutput, formatJSON, commandTool, "generate", "--api", api},
			project: projectGeneratedTool,
		}, nil
	},
}

var toolValidateDescriptor = descriptor{
	name:            "bridgectl_tool_validate",
	effect:          effectLocal,
	localValidation: true,
	tool: mcp.NewTool("bridgectl_tool_validate",
		mcp.WithDescription("Validate one tool manifest locally without contacting the deployment."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline tool manifest object.")),
	),
	invoke: manifestValidator([]string{flagOutput, formatJSON, commandTool, commandValidate}, kindTool),
}

var pluginGetDescriptor = descriptor{
	name:        "bridgectl_plugin_get",
	effect:      effectRead,
	namedLookup: true,
	tool: mcp.NewTool("bridgectl_plugin_get",
		mcp.WithDescription("Read external plugin resources from the pinned deployment. Omit name to list; provide name and version to read one. The result is a redacted inspection view."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("name", mcp.Description("Plugin name to read. Omit to list every plugin.")),
		mcp.WithString("version", mcp.Description("Exact plugin version to read. Requires name.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "version"); failure != nil {
			return invocation{}, failure
		}
		identifier, failure := ic.versionedIdentifier(args)
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandPlugin, commandGet, flagOutput, formatJSON}
		if identifier == "" {
			return invocation{argv: argv, project: projectPluginList}, nil
		}
		if !strings.Contains(identifier, "@") {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"version is required when reading one plugin",
				"Provide name@version as name and version.")
		}
		argv = append(argv, "--", identifier)
		return invocation{argv: argv, project: projectPluginItem}, nil
	},
}

var pluginValidateDescriptor = descriptor{
	name:            "bridgectl_plugin_validate",
	effect:          effectLocal,
	localValidation: true,
	tool: mcp.NewTool("bridgectl_plugin_validate",
		mcp.WithDescription("Validate one external plugin manifest locally without contacting the deployment."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline plugin manifest object.")),
	),
	invoke: manifestValidator([]string{flagOutput, formatJSON, commandPlugin, commandValidate}, kindPlugin),
}

var bindingGetDescriptor = descriptor{
	name:        "bridgectl_binding_get",
	effect:      effectRead,
	namedLookup: true,
	tool: mcp.NewTool("bridgectl_binding_get",
		mcp.WithDescription("Read external plugin bindings from the pinned deployment. Omit name to list; provide name to read one. The result is a redacted inspection view."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("name", mcp.Description("Binding name to read. Omit to list every binding.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.optionalString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandPlugin, "binding", commandGet, flagOutput, formatJSON}
		if name == "" {
			return invocation{argv: argv, project: projectBindingList}, nil
		}
		argv = append(argv, "--", name)
		return invocation{argv: argv, project: projectBindingItem}, nil
	},
}

var bindingValidateDescriptor = descriptor{
	name:            "bridgectl_binding_validate",
	effect:          effectLocal,
	localValidation: true,
	tool: mcp.NewTool("bridgectl_binding_validate",
		mcp.WithDescription("Validate one plugin binding manifest locally without contacting the deployment."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline binding manifest object.")),
	),
	invoke: manifestValidator([]string{flagOutput, formatJSON, commandPlugin, "binding", commandValidate}, kindBinding),
}

var tokenListDescriptor = descriptor{
	name:   "bridgectl_token_list",
	effect: effectRead,
	tool: mcp.NewTool("bridgectl_token_list",
		mcp.WithDescription("List ERPBridge API token metadata for the pinned deployment. Token values are never returned."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	),
	invoke: noArgumentInvocation([]string{flagOutput, formatJSON, "token", commandList}, projectTokenList),
}

var cacheStatsDescriptor = descriptor{
	name:   "bridgectl_cache_stats",
	effect: effectRead,
	tool: mcp.NewTool("bridgectl_cache_stats",
		mcp.WithDescription("Read tool cache statistics for the pinned deployment."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	),
	invoke: noArgumentInvocation([]string{flagOutput, formatJSON, "cache", "stats"}, projectCacheStats),
}

var logStatsDescriptor = descriptor{
	name:   "bridgectl_log_stats",
	effect: effectRead,
	tool: mcp.NewTool("bridgectl_log_stats",
		mcp.WithDescription("Read a bounded summary of recent ERPBridge log events: counts by level and by tool. Log messages are never returned."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	),
	invoke: noArgumentInvocation([]string{flagOutput, formatJSON, "log", "stats"}, projectLogStats),
}

// manifestValidator builds a validation invocation for one resource kind.
func manifestValidator(prefix []string, kind string) func(invocationContext, map[string]any) (invocation, *Result) {
	return func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "manifest"); failure != nil {
			return invocation{}, failure
		}
		manifest, failure := ic.prepareManifest(args, kind)
		if failure != nil {
			return invocation{}, failure
		}
		argv := append(append([]string{}, prefix...), "-f", manifest.path)
		return invocation{
			argv: argv,
			project: func([]byte) (any, error) {
				return ValidationData{
					Kind:    manifest.kind,
					Name:    manifest.name,
					Version: manifest.version,
					Valid:   true,
				}, nil
			},
		}, nil
	}
}

// projectGeneratedTool returns the generated manifest after typed credential and
// URL checks. Unsafe generation is rejected rather than redacted, because a
// redacted manifest would no longer be executable.
func projectGeneratedTool(stdout []byte) (any, error) {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return nil, errors.New("generated manifest is not a JSON object")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(trimmed), &document); err != nil {
		return nil, fmt.Errorf("decode generated manifest: %w", err)
	}
	if scanCredentialValues(document, false) {
		return nil, errors.New("generated manifest contains a literal credential value")
	}
	if endpoint, ok := generatedEndpoint(document); ok {
		if problem := endpointProblem(endpoint); problem != "" {
			return nil, errors.New(problem)
		}
	}
	return GenerateData{Manifests: []json.RawMessage{json.RawMessage(trimmed)}}, nil
}

// generatedEndpoint extracts the execution endpoint from a generated manifest.
func generatedEndpoint(document map[string]any) (string, bool) {
	spec, ok := document["spec"].(map[string]any)
	if !ok {
		return "", false
	}
	execution, ok := spec["execution"].(map[string]any)
	if !ok {
		return "", false
	}
	endpoint, ok := execution["endpoint"].(string)
	return endpoint, ok && endpoint != ""
}

// endpointProblem rejects an endpoint that could carry credentials or an
// ambiguous destination.
func endpointProblem(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "generated endpoint is not a valid URL"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "generated endpoint must use http or https"
	}
	if parsed.Host == "" {
		return "generated endpoint must include a host"
	}
	if parsed.User != nil {
		return "generated endpoint must not contain credentials in the userinfo"
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "generated endpoint must not contain a query or fragment"
	}
	return ""
}
