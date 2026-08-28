# Plan: Split `bridgectl tool generate` Output

## Goal

Add an opt-in output-directory flag to `bridgectl tool generate` so OpenAPI-generated tools are written as one file per tool, with each filename derived from the tool name. Preserve the current stdout behavior when the flag is omitted.

## Current State

- `tool generate` loads a registered API, calls `GenerateFromOpenAPI`, and serializes the complete returned tool slice to stdout as YAML or JSON; it does not write files (`internal/cli/tool.go`, `toolGenerateCmd`).
- `GenerateFromOpenAPI` creates one `mcp.Tool` per OpenAPI operation and sanitizes operation names (`internal/idp/generator.go`, `GenerateFromOpenAPI`, `sanitizeOperationName`).
- `tool apply` already accepts individual JSON/YAML files and directories, so split output can be applied without server changes (`internal/cli/tool.go`, `toolApplyCmd`, `decodeToolDocuments`).
- Existing CLI tests cover OpenAPI generation and generated YAML application (`internal/cli/tool_test.go`, `TestToolGenerateCmdUsesSelectedContextRegistry`, `TestToolApplyGeneratedYAMLFileExactlyOnce`).
- Onboarding documentation describes one generated YAML manifest and the generated-tools changelog entry states that behavior (`docs/onboarding.md`, Step 4; `CHANGELOG.md`, Unreleased Added).

## Decisions

- Add `--output-dir` to `tool generate`; when supplied, write one file per generated tool into that directory.
- Select the file extension from `-o`: `.yaml` for YAML and `.json` for JSON. The existing default/table generation mode is treated as JSON; other unsupported formats fail in directory mode with an actionable error.
- Use the exact sanitized `metadata.name` as the basename, so files are `<tool-name>.yaml` or `<tool-name>.json`.
- Create the directory if needed and overwrite matching files, consistent with generated artifact workflows. Do not change server persistence or MCP behavior.
- Keep stdout generation unchanged when `--output-dir` is absent.

## Scope

In scope:

- CLI flag, directory writing, deterministic filenames, format handling, focused tests.
- CLI help and onboarding/changelog documentation.

Out of scope:

- Changing OpenAPI-to-tool conversion rules.
- Changing `tool apply`, server storage, or MCP runtime behavior.
- Retaining generated files as an additional source of truth; reviewed manifests remain the deployment artifact.

## Tasks

- [x] Task 1: Add split-directory generation behavior and flag to `internal/cli/tool.go`. (**Seam:** `toolGenerateCmd.RunE`; **Files:** `internal/cli/tool.go`; **Verify:** `go test ./internal/cli -run 'TestToolGenerate'`)
- [x] Task 2: Add tests for YAML and JSON split output, tool-name filenames, directory creation, and unchanged stdout mode. (**Seam:** `toolGenerateCmd.RunE`; **Files:** `internal/cli/tool_test.go`; **Verify:** `go test ./internal/cli -run 'TestToolGenerate'`)
- [x] Task 3: Update CLI/onboarding documentation and the Unreleased changelog entry. (**Files:** `docs/cli/bridgectl_tool_generate.md`, `docs/onboarding.md`, `CHANGELOG.md`; **Verify:** inspect generated command help and documentation examples)
- [x] Task 4: Run repository verification. (**Files:** none; **Verify:** `make test`, `golangci-lint run ./internal/cli`, and `lens_diagnostics mode=full`; primary diagnostics are clean, with unrelated pre-existing auxiliary findings reported by the project-wide scan)

## Verification

- Without `--output-dir`, generation still emits one YAML sequence or JSON array to stdout.
- With `--output-dir ./manifests`, each generated tool is written exactly once as `./manifests/<metadata.name>.yaml` or `.json`.
- The output directory is created when absent.
- Names cannot contain path separators because they come from sanitized tool metadata names.
- Applying the generated directory with `bridgectl tool apply -f <dir>` remains supported.

## Open Questions

None. Recommended flag: `--output-dir`.
