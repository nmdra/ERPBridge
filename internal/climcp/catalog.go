package climcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
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
	commandAPI      = "api"
	commandTool     = "tool"
	commandPlugin   = "plugin"
	commandValidate = "validate"
	commandApply    = "apply"
	commandDelete   = "delete"
	commandBinding  = "binding"
	commandToken    = "token"
	selectorAll     = "all"
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
	// effectWrite changes local or remote state.
	effectWrite
	// effectProbe performs a remote probe that may cause downstream effects.
	effectProbe
)

// isSideEffecting reports whether a failure may have left a change behind.
func (e sideEffect) isSideEffecting() bool {
	return e == effectWrite || e == effectProbe
}

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
	// additionalGroup returns an extra startup group this call requires.
	additionalGroup func(args map[string]any) WriteGroup
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
	if d.additionalGroup != nil {
		if extra := d.additionalGroup(args); extra != "" && !s.Allows(extra) {
			failure := NewFailure(d.name, s.snapshot.ContextName, CodePermissionDenied,
				fmt.Sprintf("this call also requires the %q write group", extra),
				"Restart the management MCP process with every required --allow-write group.")
			return failure.callToolResult()
		}
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
		if d.effect.isSideEffecting() {
			return s.classifyWriteFailure(d, outcome, err)
		}
		return s.classifyRunFailure(d, outcome, err)
	}
	if outcome.ExitCode != 0 {
		if d.effect.isSideEffecting() {
			return s.classifyWriteFailure(d, outcome, nil)
		}
		return s.classifyExitFailure(d, outcome)
	}
	data, projectErr := inv.project(outcome.Stdout)
	if projectErr != nil {
		var probeErr *probeFailure
		if errors.As(projectErr, &probeErr) {
			failure := NewFailure(d.name, s.snapshot.ContextName, CodeProbeFailed,
				"the API probe completed and reported a failed result",
				"Check the registered endpoint and its credential before retrying.")
			failure.Error.Details = probeErr.details
			return failure
		}
		if d.effect.isSideEffecting() {
			return s.outcomeUnknown(d, "unusable_output")
		}
		return NewFailure(d.name, s.snapshot.ContextName, CodeInvalidChildOutput,
			"the command completed but its output could not be interpreted",
			"Use the bridgectl CLI directly to inspect the raw result.")
	}
	return NewSuccess(d.name, s.snapshot.ContextName, data)
}

// outcomeUnknown reports a launched side-effecting operation whose result is
// unknown. The caller must read the target back before retrying.
func (s *Server) outcomeUnknown(d descriptor, cause string) *Result {
	failure := NewFailure(d.name, s.snapshot.ContextName, CodeOutcomeUnknown,
		"the operation ended without a confirmed result",
		"Read the target resource before retrying; the change may already be applied.")
	failure.Error.Cause = cause
	return failure
}

// classifyWriteFailure maps a failed write or probe. Only a provable pre-run
// rejection reports a specific code; every other failure is an unknown outcome,
// because a launched mutation may already have been applied.
func (s *Server) classifyWriteFailure(d descriptor, outcome Outcome, err error) *Result {
	switch {
	case errors.Is(err, ErrBusy):
		return NewFailure(d.name, s.snapshot.ContextName, CodeBusy,
			"another management operation is already running",
			"Retry after the current operation finishes.")
	case errors.Is(err, ErrTimeout):
		return s.outcomeUnknown(d, "timeout")
	case errors.Is(err, ErrCancelled):
		return s.outcomeUnknown(d, "cancelled")
	case errors.Is(err, ErrOutputLimit):
		return s.outcomeUnknown(d, "output_limit")
	case err != nil:
		// The child never started, so no handler ran.
		return NewFailure(d.name, s.snapshot.ContextName, CodeInternal,
			"the management command could not be started",
			"Run bridgectl directly to diagnose the local environment.")
	}

	if code, ok := childErrorCode(outcome.Stdout, outcome.Stderr); ok && code == childCodeContextChanged {
		// The child refused before its handler ran.
		return NewFailure(d.name, s.snapshot.ContextName, CodeContextChanged,
			"the session selection no longer matches the saved configuration",
			"Restart the management MCP process to adopt the changed context, endpoint, or credential.")
	}
	if outcome.ExitCode == exitCodeBadArguments {
		// The child rejected its own argument vector before running a handler.
		return NewFailure(d.name, s.snapshot.ContextName, CodeInvalidArguments,
			"the child rejected its arguments",
			"Correct the input and retry.")
	}
	return s.outcomeUnknown(d, "child_failed")
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

// childCodeContextChanged is the recognized child precondition code.
const childCodeContextChanged = "MCP_CONTEXT_CHANGED"

// exitCodeBadArguments is the CLI exit code for rejected arguments.
const exitCodeBadArguments = 2

// maxTargetBytes bounds a canonical confirmation target.
const maxTargetBytes = 256

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
	apiRegisterDescriptor,
	apiSetCredentialRefDescriptor,
	apiTestDescriptor,
	toolApplyDescriptor,
	toolDeleteDescriptor,
	pluginApplyDescriptor,
	pluginDeleteDescriptor,
	bindingApplyDescriptor,
	bindingDeleteDescriptor,
	tokenRevokeDescriptor,
	cacheFlushDescriptor,
	contextSetDescriptor,
}

// registerMethods and registerAuthTypes are the accepted registration values.
var (
	registerMethods   = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	registerAuthTypes = []string{"api-key", "bearer", "basic"}
)

const (
	cacheSelectorTool   = "tool"
	cacheSelectorModule = "module"
)

var cacheSelectors = []string{cacheSelectorTool, cacheSelectorModule, selectorAll}

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

// requireTarget validates the two confirmation arguments against the pinned
// context and one canonical target.
func (ic invocationContext) requireTarget(args map[string]any, canonical string) *Result {
	confirmContext, failure := ic.requiredString(args, "confirmContext", maxIdentifierBytes)
	if failure != nil {
		return failure
	}
	if confirmContext != ic.contextName {
		return ic.fail(CodeConfirmationMismatch,
			"confirmContext does not match the context pinned to this session",
			"Repeat the pinned context name exactly.")
	}
	target, failure := ic.requiredString(args, "confirmTarget", maxTargetBytes)
	if failure != nil {
		return failure
	}
	if target != canonical {
		return ic.fail(CodeConfirmationMismatch,
			"confirmTarget does not match the requested target",
			"Send the canonical target exactly as documented for this tool.")
	}
	return nil
}

// optionalBool reads a validated optional boolean argument.
func (ic invocationContext) optionalBool(args map[string]any, key string) (bool, *Result) {
	raw, present := args[key]
	if !present || raw == nil {
		return false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, ic.fail(CodeInvalidArguments,
			fmt.Sprintf("%s must be a boolean", key),
			"Correct the argument and retry.")
	}
	return value, nil
}

// headerTokenProblem reports why a value is not usable as an HTTP header name.
func headerTokenProblem(value string) string {
	if value == "" {
		return "must not be empty"
	}
	if len(value) > maxIdentifierBytes {
		return fmt.Sprintf("must be at most %d bytes", maxIdentifierBytes)
	}
	for _, r := range value {
		if !isHeaderTokenRune(r) {
			return "must be a valid HTTP header name"
		}
	}
	return ""
}

// isHeaderTokenRune reports whether a rune may appear in an HTTP header field
// name (RFC 9110 token).
func isHeaderTokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

// commandCompleted builds the standard write summary.
func commandCompleted(target string) WriteData {
	return WriteData{Target: target, Effect: effectCommandCompleted, Verified: false}
}

// targetOf composes the canonical name@version target used in confirmations.
func targetOf(name, version string) string {
	if version == "" {
		return name
	}
	return name + "@" + version
}

// applyInvocation builds a resource-apply invocation. The caller confirms the
// target derived from the submitted manifest.
func applyInvocation(prefix []string, kind string) func(invocationContext, map[string]any) (invocation, *Result) {
	return func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "manifest", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		manifest, failure := ic.prepareManifest(args, kind)
		if failure != nil {
			return invocation{}, failure
		}
		target := targetOf(manifest.name, manifest.version)
		if failure := ic.requireTarget(args, target); failure != nil {
			return invocation{}, failure
		}
		argv := append(append([]string{}, prefix...), flagFile, manifest.path)
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(target), nil },
		}, nil
	}
}

var apiRegisterDescriptor = descriptor{
	name:          "bridgectl_api_register",
	requiredGroup: GroupRegistry,
	effect:        effectWrite,
	additionalGroup: func(args map[string]any) WriteGroup {
		if force, _ := args["force"].(bool); force {
			return GroupDestructive
		}
		return ""
	},
	tool: mcp.NewTool("bridgectl_api_register",
		mcp.WithDescription("Register an ERP API in the local registry for the pinned context. Replacing an existing registration also requires the destructive group."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Registration name.")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Absolute ERP endpoint URL without credentials in the userinfo, query, or fragment.")),
		mcp.WithString("module", mcp.Required(), mcp.Description("ERP module this API belongs to.")),
		mcp.WithString("description", mcp.Required(), mcp.Description("Human-readable description. Never place a secret here.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the registration name.")),
		mcp.WithString("method", mcp.Enum(registerMethods...), mcp.Description("HTTP method. Defaults to GET.")),
		mcp.WithString("authType", mcp.Enum("", "api-key", "bearer", "basic"), mcp.Description("Authentication type. Defaults to api-key.")),
		mcp.WithString("authHeader", mcp.Description("Header name that carries the credential. Defaults to X-API-Key.")),
		mcp.WithString("credentialRef", mcp.Description("Logical credential reference, never a credential value.")),
		mcp.WithString("credentialSource", mcp.Enum("", "env", "file"), mcp.Description("Credential source. Defaults to env.")),
		mcp.WithBoolean("force", mcp.Description("Replace an existing registration with the same name. Also requires the destructive group.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		allowed := []string{"name", "url", "module", "description", "method", "authType", "authHeader", "credentialRef", "credentialSource", "force", "confirmContext", "confirmTarget"}
		if failure := ic.rejectUnknown(args, allowed...); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, name); failure != nil {
			return invocation{}, failure
		}
		module, failure := ic.requiredString(args, "module", maxModuleBytes)
		if failure != nil {
			return invocation{}, failure
		}
		description, failure := ic.requiredString(args, "description", maxDescriptionBytes)
		if failure != nil {
			return invocation{}, failure
		}
		endpoint, failure := ic.requiredString(args, "url", maxURIBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if problem := endpointProblem(endpoint); problem != "" {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"url "+problem,
				"Provide an absolute ERP endpoint URL without embedded credentials.")
		}
		method := "GET"
		if value, failure := ic.optionalString(args, "method", maxIdentifierBytes); failure != nil {
			return invocation{}, failure
		} else if value != "" {
			method = strings.ToUpper(value)
		}
		if !slices.Contains(registerMethods, method) {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"method is not one of the accepted HTTP methods",
				"Use one of "+strings.Join(registerMethods, ", ")+".")
		}
		authType, failure := ic.optionalString(args, "authType", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if authType == "" {
			authType = "api-key"
		}
		if !slices.Contains(registerAuthTypes, authType) {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"authType is not one of the accepted values",
				"Use one of "+strings.Join(registerAuthTypes, ", ")+".")
		}
		authHeader := "X-API-Key"
		if value, failure := ic.optionalString(args, "authHeader", maxIdentifierBytes); failure != nil {
			return invocation{}, failure
		} else if value != "" {
			authHeader = value
		}
		if problem := headerTokenProblem(authHeader); problem != "" {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"authHeader "+problem,
				"Provide a header name, never a header value.")
		}
		credentialRef, failure := ic.optionalString(args, "credentialRef", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		credentialSource, failure := ic.optionalString(args, "credentialSource", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		force, failure := ic.optionalBool(args, "force")
		if failure != nil {
			return invocation{}, failure
		}

		argv := []string{flagOutput, formatJSON, commandAPI, "register",
			"--name=" + name, "--url=" + endpoint, "--method=" + method, "--module=" + module,
			"--description=" + description, "--auth-type=" + authType, "--auth-header=" + authHeader,
		}
		if credentialRef != "" {
			argv = append(argv, "--credential-ref="+credentialRef)
		}
		if credentialSource != "" {
			argv = append(argv, "--credential-source="+credentialSource)
		}
		if force {
			argv = append(argv, "--force")
		}
		return invocation{
			argv: argv,
			project: func(stdout []byte) (any, error) {
				data := commandCompleted(name)
				var wire wireRegistration
				if err := decodeChildJSON(stdout, &wire); err != nil {
					return nil, err
				}
				replaced := wire.Replaced
				data.Replaced = &replaced
				return data, nil
			},
		}, nil
	},
}

var apiSetCredentialRefDescriptor = descriptor{
	name:          "bridgectl_api_set_credential_ref",
	requiredGroup: GroupRegistry,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_api_set_credential_ref",
		mcp.WithDescription("Set the logical credential reference for a registered API. The reference is a name, never a credential value."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Registered API name.")),
		mcp.WithString("credentialRef", mcp.Required(), mcp.Description("Logical credential reference.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the API name.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "credentialRef", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, name); failure != nil {
			return invocation{}, failure
		}
		credentialRef, failure := ic.requiredString(args, "credentialRef", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandAPI, "set-credential-ref",
			"--credential-ref=" + credentialRef, separator, name}
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(name), nil },
		}, nil
	},
}

var apiTestDescriptor = descriptor{
	name:          "bridgectl_api_test",
	requiredGroup: GroupProbe,
	effect:        effectProbe,
	tool: mcp.NewTool("bridgectl_api_test",
		mcp.WithDescription("Run the server-side probe for a registered API. The probe sends a real request to the ERP endpoint, so it may cause downstream effects."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Registered API name to probe.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the API name.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, name); failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandAPI, "test", separator, name}
		return invocation{argv: argv, project: projectAPITest}, nil
	},
}

var toolApplyDescriptor = descriptor{
	name:          "bridgectl_tool_apply",
	requiredGroup: GroupResources,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_tool_apply",
		mcp.WithDescription("Apply one tool manifest to the pinned deployment. Applying a manifest changes future tool execution."),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline tool manifest object.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the manifest name, or name@version.")),
	),
	invoke: applyInvocation([]string{flagOutput, formatJSON, commandTool, commandApply}, kindTool),
}

var toolDeleteDescriptor = descriptor{
	name:          "bridgectl_tool_delete",
	requiredGroup: GroupDestructive,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_tool_delete",
		mcp.WithDescription("Remove a tool from the pinned deployment. Without hard the tool is deactivated; with hard it is permanently deleted."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Tool name.")),
		mcp.WithString("version", mcp.Required(), mcp.Description("Tool version.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal name@version.")),
		mcp.WithBoolean("hard", mcp.Description("Permanently delete instead of deactivating.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "version", "hard", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		version, failure := ic.requiredString(args, "version", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		target := targetOf(name, version)
		if failure := ic.requireTarget(args, target); failure != nil {
			return invocation{}, failure
		}
		hard, failure := ic.optionalBool(args, "hard")
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandTool, commandDelete}
		if hard {
			argv = append(argv, "--hard", "--yes")
		}
		argv = append(argv, separator, name, version)
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(target), nil },
		}, nil
	},
}

var pluginApplyDescriptor = descriptor{
	name:          "bridgectl_plugin_apply",
	requiredGroup: GroupResources,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_plugin_apply",
		mcp.WithDescription("Apply one external plugin manifest to the pinned deployment. Applying a manifest changes future plugin execution."),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline plugin manifest object.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the manifest name, or name@version.")),
	),
	invoke: applyInvocation([]string{flagOutput, formatJSON, commandPlugin, commandApply}, kindPlugin),
}

var pluginDeleteDescriptor = descriptor{
	name:          "bridgectl_plugin_delete",
	requiredGroup: GroupDestructive,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_plugin_delete",
		mcp.WithDescription("Remove an external plugin from the pinned deployment. Without hard the plugin is deactivated; with hard it is permanently deleted."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name.")),
		mcp.WithString("version", mcp.Required(), mcp.Description("Plugin version.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal name@version.")),
		mcp.WithBoolean("hard", mcp.Description("Permanently delete instead of deactivating.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "version", "hard", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		version, failure := ic.requiredString(args, "version", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if strings.Contains(name, "@") || strings.Contains(version, "@") {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"name and version must not contain '@'",
				"Provide the name and version as separate arguments.")
		}
		target := targetOf(name, version)
		if failure := ic.requireTarget(args, target); failure != nil {
			return invocation{}, failure
		}
		hard, failure := ic.optionalBool(args, "hard")
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandPlugin, commandDelete}
		if hard {
			argv = append(argv, "--hard", "--yes")
		}
		argv = append(argv, separator, target)
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(target), nil },
		}, nil
	},
}

var bindingApplyDescriptor = descriptor{
	name:          "bridgectl_binding_apply",
	requiredGroup: GroupResources,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_binding_apply",
		mcp.WithDescription("Apply one plugin binding manifest to the pinned deployment. A binding connects a plugin to a tool's processing phase."),
		mcp.WithObject("manifest", mcp.Required(), mcp.Description("One inline binding manifest object.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the manifest binding name.")),
	),
	invoke: applyInvocation([]string{flagOutput, formatJSON, commandPlugin, commandBinding, commandApply}, kindBinding),
}

var bindingDeleteDescriptor = descriptor{
	name:          "bridgectl_binding_delete",
	requiredGroup: GroupDestructive,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_binding_delete",
		mcp.WithDescription("Remove a plugin binding from the pinned deployment. Without hard the binding is deactivated; with hard it is permanently deleted."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Binding name.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the binding name.")),
		mcp.WithBoolean("hard", mcp.Description("Permanently delete instead of deactivating.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "hard", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, name); failure != nil {
			return invocation{}, failure
		}
		hard, failure := ic.optionalBool(args, "hard")
		if failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandPlugin, commandBinding, commandDelete}
		if hard {
			argv = append(argv, "--hard", "--yes")
		}
		argv = append(argv, separator, name)
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(name), nil },
		}, nil
	},
}

var tokenRevokeDescriptor = descriptor{
	name:          "bridgectl_token_revoke",
	requiredGroup: GroupDestructive,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_token_revoke",
		mcp.WithDescription("Revoke one ERPBridge API token by identifier. Revoking a token cannot be undone."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Token identifier.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the token identifier.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "id", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		id, failure := ic.requiredString(args, "id", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, id); failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, commandToken, "revoke", separator, id}
		return invocation{
			argv:    argv,
			project: func([]byte) (any, error) { return commandCompleted(id), nil },
		}, nil
	},
}

var cacheFlushDescriptor = descriptor{
	name:          "bridgectl_cache_flush",
	requiredGroup: GroupDestructive,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_cache_flush",
		mcp.WithDescription("Delete tool cache entries for the pinned deployment. Choose exactly one selector: tool, module, or all."),
		mcp.WithString("selector", mcp.Required(), mcp.Enum(cacheSelectors...), mcp.Description("What to flush.")),
		mcp.WithString("name", mcp.Description("Tool or module name. Required for the tool and module selectors and forbidden for all.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal tool:<name>, module:<name>, or all.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "selector", "name", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		selector, failure := ic.requiredString(args, "selector", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if !slices.Contains(cacheSelectors, selector) {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"selector must be tool, module, or all",
				"Choose exactly one selector.")
		}
		name, failure := ic.optionalString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if selector == selectorAll && name != "" {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"name is forbidden with the all selector",
				"Omit name, or choose the tool or module selector.")
		}
		if selector != selectorAll && name == "" {
			return invocation{}, ic.fail(CodeInvalidArguments,
				"name is required for this selector",
				"Provide the tool or module name.")
		}
		target := selector
		if name != "" {
			target = selector + ":" + name
		}
		if failure := ic.requireTarget(args, target); failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, "cache", "flush"}
		switch selector {
		case cacheSelectorTool:
			argv = append(argv, separator, name)
		case cacheSelectorModule:
			argv = append(argv, "--module="+name)
		default:
			argv = append(argv, "--all")
		}
		return invocation{
			argv: argv,
			project: func(stdout []byte) (any, error) {
				var wire wireFlush
				if err := decodeChildJSON(stdout, &wire); err != nil {
					return nil, err
				}
				return CacheFlushData{Target: target, Deleted: wire.Deleted, Status: wire.Status, Verified: false}, nil
			},
		}, nil
	},
}

var contextSetDescriptor = descriptor{
	name:          "bridgectl_context_set",
	requiredGroup: GroupContext,
	effect:        effectWrite,
	tool: mcp.NewTool("bridgectl_context_set",
		mcp.WithDescription("Change the saved default bridgectl context. This does not retarget the running session, which keeps its pinned context."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of an already saved context.")),
		mcp.WithString("confirmContext", mcp.Required(), mcp.Description("Must equal the context pinned to this session.")),
		mcp.WithString("confirmTarget", mcp.Required(), mcp.Description("Must equal the requested context name.")),
	),
	invoke: func(ic invocationContext, args map[string]any) (invocation, *Result) {
		if failure := ic.rejectUnknown(args, "name", "confirmContext", "confirmTarget"); failure != nil {
			return invocation{}, failure
		}
		name, failure := ic.requiredString(args, "name", maxIdentifierBytes)
		if failure != nil {
			return invocation{}, failure
		}
		if failure := ic.requireTarget(args, name); failure != nil {
			return invocation{}, failure
		}
		argv := []string{flagOutput, formatJSON, "context", "set", separator, name}
		pinned := ic.contextName
		return invocation{
			argv: argv,
			project: func([]byte) (any, error) {
				return ContextSelectData{SavedDefault: name, ActiveContext: pinned, Verified: false}, nil
			},
		}, nil
	},
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
	document := make(map[string]any)
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
