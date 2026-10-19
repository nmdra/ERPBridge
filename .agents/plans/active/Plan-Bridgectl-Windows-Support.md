# Plan: Native bridgectl Windows support

Status: Active, implementation approved. The user selected native CLI and PowerShell support, with no Linux behavior changes. The user explicitly approved local implementation of all tasks while native Windows and interactive gates remain open for CI/manual validation. Implement tasks in order, but do not mark tasks or the plan complete until their deferred gates pass.

## Goal

Make the existing Windows CLI artifacts a tested, documented way to manage a remote ERPBridge deployment. Protect per-context registry isolation on normal Windows filesystems. Cover installation, configuration, local validation, skill installation, management MCP, and the local web Console without requiring Bash, Make, Docker, or administrator access for ordinary CLI use.

## Current State

### Existing support

- Windows distribution is already configured. GoReleaser builds bridgectl for Windows amd64 and arm64 and packages Windows binaries in ZIP files. It also generates `checksums.txt` (`.goreleaser.yaml:32-45,67-81,108-109`). Do not describe this work as adding the first Windows binary.
- Server release targets remain Linux-only (`.goreleaser.yaml:12-24`). That is separate from using the Windows CLI against a remote deployment.
- Config and registry paths use `os.UserHomeDir` and `filepath`, not hard-coded Unix home paths (`internal/config/config.go:171-173`; `internal/idp/registry.go:46-58`). Skill destinations also use these portable APIs (`internal/cli/skill.go:47-91`). Preserve the existing `%USERPROFILE%\.bridgectl` layout.
- Console launch already selects `rundll32 url.dll,FileProtocolHandler` on Windows and passes the generated URL as an argument, not shell text (`internal/web/browser.go:9-23`). `web --no-open` and `web --url` bypass browser launch (`internal/cli/web.go:80-82,104-108`). There is no demonstrated Windows browser failure to fix speculatively.
- Management MCP launches the pinned executable with an argument vector rather than a shell (`internal/climcp/exec.go:102-111`; `internal/climcp/server.go:85-94`). Preserve its read-only defaults, output redaction, pinned session, and unknown-write-outcome contract.

### Confirmed source gaps and platform risks

1. **Registry isolation risk, high priority:** Context validation permits `prod` and `Prod` as distinct keys, and accepts names such as `CON` and `NUL.test` (`internal/config/config.go:20-28`). Registry filenames are formed directly from the name (`internal/idp/registry.go:52-58`). Microsoft documents default case-insensitive naming and reserved device names, including names followed by extensions. These source rules do not preserve context isolation on a typical Windows filesystem. Native reproduction is still required; it was not performed during this audit.
2. **No native Windows CI:** Both inspected workflows use Ubuntu (`.github/workflows/console.yml:27-30`; `.github/workflows/release.yml:9-13,26-28`). Cross-compilation does not exercise config replacement, child processes, cancellation, or filesystems.
3. **Unsafe Windows test isolation:** Fixtures set `HOME`, but Windows home resolution uses `USERPROFILE`. Examples include `internal/config/context_write_test.go:11-25`, `internal/config/config_test.go:18-20,42-49`, `internal/idp/registry_test.go:17-25`, `internal/cli/mcp_test.go:47-66`, and `internal/climcp/stdio_integration_test.go:33-51`. A Windows job must fix this before running those tests against a real user profile.
4. **Unix-only test assumptions:** The fake Go builder is a shell script (`internal/climcp/build_environment_test.go:40-47`). The real CLI fixture names its output `bridgectl` without `.exe` (`internal/climcp/catalog_child_test.go:32-56`). Signal acceptance sends SIGTERM through `Process.Signal` (`internal/climcp/stdio_integration_test.go:534-555`). These are test portability problems, not proof of corresponding production failures.
5. **Privacy claims require platform qualification:** Config replacement calls `Chmod(0600)` and then `os.Rename` (`internal/config/context_write.go:91-152`); the existing test requires exact Unix permission bits (`internal/config/context_write_test.go:84-90`). Go documents that Windows Chmod changes the read-only attribute, not Unix access permissions. Rename is not guaranteed atomic on non-Unix systems. Do not promise owner-only Windows access or atomic replacement from these calls alone.
6. **Missing Windows onboarding:** Source install/build instructions use Go or Make and Bash examples (`README.md:95-106`; `docs/onboarding.md:86-98`). MCP examples use Unix executable paths (`docs/bridgectl-mcp.md:270-304`). A PowerShell completion reference exists (`docs/cli/bridgectl_completion_powershell.md:1-18`), but is not a complete installation workflow. The existing asset smoke script requires Bash (`scripts/verify-console-assets.sh:1-8,24-55`).

### Primary platform references

- Windows filename rules, reserved names, and default case behavior: https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file
- Go home resolution: https://pkg.go.dev/os#UserHomeDir
- Windows Chmod semantics: https://pkg.go.dev/os#Chmod
- Rename limitations: https://pkg.go.dev/os#Rename
- Windows interrupt behavior and shutdown limitations: https://pkg.go.dev/os/signal#hdr-Windows
- Windows `Process.Signal` limitation: https://pkg.go.dev/os#Process.Signal

Facts above are separate from the proposed contracts below. Browser launch, native filesystem behavior, and console control delivery were not tested on Windows in this planning session.

## Decisions

### Hard requirement: no Linux behavior changes

The user requires this work not to affect current Linux system behavior. This is an acceptance constraint, not a best-effort goal.

- Isolate Windows production rules with Go platform-specific files/build constraints. Any new non-Windows validation hook must be a no-op. Shared call-site wiring must preserve Linux results and side effects exactly.
- Preserve Linux config/registry/skill paths, accepted context names and case sensitivity, environment precedence, file modes, replacement and locking behavior, signals, browser launch, CLI flags, exit codes, stdout/stderr, MCP protocol/security contracts, and server behavior. No Linux data migration or configuration change is permitted.
- Keep Linux builds, archive names/content policy, Makefile commands, shell scripts, existing CI jobs, and deployment configuration unchanged. Add Windows CI in a separate workflow. Do not alter shared GoReleaser settings as part of this plan.
- Test-only portability edits are allowed, but must retain Linux assertions and coverage. Windows-specific environment case folding, USERPROFILE overrides, permission adaptations, executable suffixes, and signal exclusions must not leak into Linux runtime behavior. Keep POSIX signal and permission tests active on Linux.
- Record a Linux baseline at the approved starting revision, using isolated fixtures, then compare the same command, filesystem, and MCP contracts after changes. A green test suite alone does not prove unchanged behavior.
- Stop and request a plan amendment if a proposed fix requires changing Linux behavior or shared release/deployment settings. Do not accept a Linux regression as a Windows compatibility trade-off.

### Support boundary

Keep Windows amd64 and arm64 artifacts and archive names unchanged. Require native x64 runtime CI. Keep ARM64 cross-build coverage and label ARM64 runtime validation as unverified until tested on actual ARM64 hardware or an approved runner. Do not claim x64 CI proves ARM64 runtime support.

Use PowerShell 7 for automated shell smoke checks. Document simple install/config commands that also work in Windows PowerShell 5.1, including explicit UTF-8 output. Do not rely on newer native-argument passing behavior for inline manifests: use manifest files and direct executable invocation. Do not add Winget, Chocolatey, Scoop, MSI, Authenticode, or a download-and-execute installer in this workstream.

### Fail closed for unsafe Windows registry identities

Preserve current context spelling and existing paths. Do not lowercase names, hash filenames, migrate registries, or globally tighten the Linux naming contract.

On Windows:

- Reject device-name stems case-insensitively before forming a registry path, including a device stem followed by a dot, such as `NUL.test`. The current ASCII-only grammar excludes superscript digit variants, but tests must explain this boundary.
- Reject configuration containing context keys that differ only by case before registry access or MCP child execution. Detect all ambiguous pairs regardless of which context is selected. Give a safe message that asks the user to choose distinct names. Do not print endpoints or credentials.
- Apply the same ambiguity check to the raw saved document before `SetCurrentContext` writes it. Guard registry path construction independently against device names so direct callers cannot bypass that protection.
- Do not rename or remove existing files after rejection. Recovery is explicit manual configuration and registry backup/rename guidance, never automatic merging.

This is a deliberate Windows-only rejection of previously accepted unsafe configurations. Unix behavior must remain unchanged. A new universal naming policy or registry migration requires separate approval.

### Filesystem and credential policy

Preserve environment-based credentials. Document that inherited Windows directory ACLs govern local access; 0600 is not an owner-only ACL guarantee. Keep Unix permission assertions on Unix. On Windows, test content preservation, error paths, cleanup, and refusal behavior rather than pretending Unix mode bits prove privacy.

Test replacement of an existing config and registry on native Windows. Failed replacement must return an error, keep the original readable and unchanged, and remove temporary files where removal is possible. Do not remove the destination as a replacement workaround. Do not add retries or a new persistence abstraction without a demonstrated failure and a reviewed contract. Custom DACL enforcement, encrypted storage, concurrent-writer redesign, and crash-atomic guarantees are outside scope.

### Test and CI policy

Keep tests fixture-only. Fix test homes and inherited environment filtering before enabling native execution. Preserve the host build environment for Go compilation while isolating runtime children. Environment key filtering must be case-insensitive on Windows and retain needed host variables such as SystemRoot, PATH, TEMP, and TMP. Retain case-sensitive environment matching on Linux; Windows fixture adjustments must not change Linux environment semantics.

Replace the shell fake-builder fixture with a small Go helper executable, built using the real Go tool and host build environment before substituting PATH. Use `.exe` for Windows executable fixtures. Do not skip MCP acceptance broadly. Keep the POSIX SIGTERM process test on supported platforms only, retain EOF and protocol cancellation tests on Windows, and record interactive Windows Ctrl+C verification separately. `Process.Signal(os.Interrupt)` is not a portable replacement.

Use a focused Windows package suite, not `go test ./...` as a promise of full Windows server development. Keep existing Linux project tests and frontend gates unchanged. Build production frontend assets before Windows binary smoke tests. Do not weaken Console security or log its capability URL in CI output.

## Scope

In scope: native CLI tests, safe Windows context identities, x64 runtime CI, both Windows cross-build targets, fixture-only command/MCP/Console checks, ZIP-based PowerShell installation guidance, public docs synchronization, and qualified platform claims.

Out of scope: native server hosting, Windows services, Docker/Compose bootstrap porting, a Makefile rewrite, WSL integration, package-manager publication, new CLI features, a new config location, automatic migrations, live ERP probes, deployment changes, dependency upgrades, release publication, and operating-system-wide security settings.

## Tasks

Execute sequentially after approval and promotion to `active/`. Before task 1, record the starting revision and Linux contract baseline required above, and run the existing focused suite with `-count=1` plus `make test`. Use isolated temporary homes and local fixtures only; never read or write the operator's existing configuration. Baseline failures must be recorded and resolved through scope review, not hidden. Use red-green-refactor for Windows behavior changes. One task equals one atomic Conventional Commit. Update relevant in-repo docs and `CHANGELOG.md` Unreleased in each behavior-changing commit; synchronize public user-facing changes in corresponding commits in `../erpbridge-docs`.

- [ ] **1. Make the focused test suite safe and portable on Windows.**
  - **Files:** new `internal/testutil/home.go` and `home_test.go`; home fixtures in `internal/config/{config_test,context_write_test}.go`, `internal/idp/registry_test.go` and `schema_constraints_integration_test.go`, `internal/cli/{api_test,context_test,empty_list_test,mcp_test,skill_test,tool_test}.go`, `internal/web/topology_test.go`, and `internal/climcp/{catalog_child_test,stdio_integration_test,build_environment_test,catalog_test,exec_test}.go`; `tools/bridgectl/main_test.go`; new `internal/climcp/testdata/fakego/main.go`. The three additional test files were approved after the default-home/mode audit found Windows isolation gaps.
  - **Seam:** existing in-process home fixtures, `runCLIProcessIn`, `acceptanceEnvironment`, `realCLI`, and fresh-process builder regression. Set both HOME and USERPROFILE to fixture directories on Windows. Assert `os.UserHomeDir()` resolves inside the fixture, and test case-variant inherited environment entries. Preserve compile/runtime environment separation.
  - Replace the shell-only builder, use platform-correct executable names, scope POSIX signal checks precisely, and make Unix mode checks platform-aware. Adapt `TestResolveSkillDestination` to native temporary paths and compare quoted error paths using the same quoting contract as production. Replace the registry test's `Mkdir(..., 0400)` unwritable-directory assumption with deterministic failure injection at the existing registry write/rename seam. Retain the error assertions rather than skipping them. Explicitly isolate `TestNewRegistry/empty path uses home dir`, which currently resolves a default home path without a fixture. Audit every default-path test in the focused packages, not only existing HOME assignments. Add a sentinel outside the fixture and assert it stays untouched, but do not treat that alone as proof that the real profile was never read.
  - **Verify:** first run the bootstrap gate `go test -count=1 ./internal/testutil -run '^TestHomeIsolation'` on Linux and native Windows. Implement this test family to check current-process and self-reexecuted child `os.UserHomeDir`, inherited case-variant environment filtering, preservation of required host variables, and synthetic credential removal without importing CLI packages or reading real config. Review all focused default-path fixtures before proceeding. Only after that gate passes, run `go test -count=1 ./internal/testutil ./internal/config ./internal/idp ./internal/bridgeclient ./internal/cli ./internal/web ./internal/climcp ./tools/bridgectl` on both platforms. Do not run the acceptance suite against a real Windows profile before fixture isolation is verified. If unrelated package failures appear, report exact failures and request a bounded amendment rather than broadening the plan.

- [ ] **2. Prevent Windows context registry aliases and reserved paths.**
  - **Files:** `internal/config/config.go`, `context_write.go`, their tests; new `internal/config/context_names_windows.go` and `context_names_other.go`; `internal/idp/registry.go`, `registry_test.go`; `docs/onboarding.md`; `CHANGELOG.md`; public `docs/bridgectl/overview.mdx` and `CHANGELOG.md`.
  - **Seam:** `Load`, raw-document `SetCurrentContext`, and `ContextRegistryPath`. Add OS-specific validation helpers without changing public function signatures. Test `prod`/`Prod`, `CON`, `con`, `NUL.test`, `COM1`, `LPT9`, safe lookalikes such as `console`/`com10`, and ordinary existing names. Device rejection applies to registry paths; ambiguity rejection applies to configured key sets before selection/write. Test original config/registry bytes and absence of newly created registry/temp files after refusal.
  - Test Windows refusal and unchanged Unix acceptance. Native Windows tests must show that distinct allowed contexts keep distinct registry data. Document manual recovery without merging or deleting state.
  - **Verify:** `go test -count=1 ./internal/config ./internal/idp ./internal/cli ./internal/climcp` on Linux and Windows; `npm --prefix ../erpbridge-docs run build`; `git diff --check` in both repositories.

- [ ] **3. Add native command and Console smoke coverage.**
  - **Files:** new `scripts/test-bridgectl-windows.ps1`; `internal/climcp/{catalog_child_test,stdio_integration_test}.go`; `internal/config/context_write_test.go` and new `context_replace_failure_test.go`; `internal/idp/registry_test.go`; `internal/cli/skill_test.go`; `docs/web-console.md`; `CHANGELOG.md`; public `docs/erpbridge/console.mdx` and `CHANGELOG.md` if documented behavior changes.
  - **Seam:** real executable at a quoted path containing spaces, Go httptest deployment fixtures, existing MCP stdio acceptance, and HTTP against the local Console. Reuse acceptance cases for initialize/list/read, redaction, context pinning, EOF, read cancellation, interrupted-write `outcome_unknown`, and survival after cancellation. All writes go to local fixtures only.
  - PowerShell script takes an explicit binary path, sets terminating errors and checks every native exit code, and uses isolated HOME/USERPROFILE/TEMP/TMP state. Before every runtime launch, clear inherited `BRIDGE_*` variables, the internal MCP child snapshot variable identified by `climcp.ChildSnapshotEnv`, and `LOG_TO_STDERR`, using case-insensitive key matching. Add only explicit fixture values. Preserve required Windows host variables. Verify filtering with synthetic credential/snapshot canaries. Save and restore both original presence and values of all changed environment entries in `finally`. Test version, PowerShell completion generation, config selection, skill install/force replacement, and invalid-command exit/stream behavior. Use a temporary path with spaces and Unicode. Install only inside temporary destinations.
  - Start `web --no-open`, capture its output privately, parse the generated URL without printing the capability, and check the embedded index plus referenced JS/CSS return expected content rather than the fallback sentinel. Bound readiness and request waits. Always terminate/reap the process and remove owned files. No upstream request is necessary for this check.
  - Add native filesystem assertions for successful existing-file replacement, simulated replacement failure with original preservation, and cleanup. Do not assert crash atomicity or owner-only ACLs. Retain platform-independent security tests.
  - **Verify:** `npm ci --prefix web`; `npm run build --prefix web`; `go build -o <temporary-dir>/bridgectl.exe ./tools/bridgectl` on Windows; `pwsh -NoProfile -File scripts/test-bridgectl-windows.ps1 -BinaryPath <temporary-dir>/bridgectl.exe`; focused package suite from task 1 on Linux and Windows. Native interactive check: default browser opens the Console; Ctrl+C ends `web` and an idle MCP process without leaving child processes. Record Windows and PowerShell versions. Automated coverage can be green while this interactive check remains pending; do not mark the task complete without recording it.

- [ ] **4. Enforce Windows compatibility in CI without changing release publication.**
  - **Files:** new `.github/workflows/bridgectl-windows.yml` only. Existing Linux workflows and `.goreleaser.yaml` remain unchanged. A packaging defect that requires changing shared release configuration must be reported for separate approval.
  - **Seam:** native `windows-latest` job with Node/Go versions matching current project config, and a Linux cross-build job. Trigger PR/main checks for Go source and module changes, embedded skills, frontend/asset changes, scripts, release config, and the workflow itself. Use read-only repository permissions and pinned action revisions consistent with existing workflows.
  - Build frontend assets, run the complete focused package suite without `-short`, build `.exe`, and execute task 3's PowerShell smoke script. On Linux cross-build both Windows amd64 and arm64 with CGO disabled. Keep the server Linux-only. Check existing Windows archive naming and executable inclusion without changing artifacts or publishing anything.
  - **Verify:** both new workflow jobs green on the implementation PR; `goreleaser check`; review workflow triggers against each input category. Inspect Windows ZIP contents from a disposable snapshot build or existing local artifacts. Do not run the all-in-one release command, sign assets, or overwrite the existing `dist/` tree. Missing interactive Windows evidence or ARM64 runtime evidence must remain visible, not inferred from CI.

- [ ] **5. Publish a tested PowerShell-first installation and operation guide.**
  - **Files:** new `docs/windows.md`; links in `README.md` and `docs/README.md`; `docs/onboarding.md`, `docs/environment-variables.md`, `docs/bridgectl-mcp.md`, `docs/web-console.md`, `CHANGELOG.md`; public new `docs/bridgectl/windows.mdx`, `docs/bridgectl/{overview,mcp}.mdx`, `CHANGELOG.md`, and `sidebars.ts` only if required for navigation.
  - **Seam:** clean non-admin Windows user following the guide against local fixtures or an already configured remote deployment, without making a live probe or mutation. Cover x64 versus ARM64 ZIP selection, a concrete versioned release URL pattern, SHA-256 comparison against that release's checksum entry, `Expand-Archive`, direct invocation with `&` and a quoted `.exe` path, and optional user PATH setup that preserves existing entries. Explain that checksums downloaded from the same location detect corruption, not independent publisher authenticity.
  - Document `%USERPROFILE%\.bridgectl\config.yaml` and registries, `${BRIDGE_API_TOKEN}` as literal YAML text, `$env:BRIDGE_API_TOKEN` for runtime injection without a literal secret example, UTF-8 file creation, manifest-file commands, completion loading through the current PowerShell session/profile without changing machine execution policy, skill destinations, Console `--no-open`, and troubleshooting.
  - Add Windows MCP TOML/JSON path escaping examples and direct executable/args arrays. Keep secrets outside client files and stdout. Explain process restart after context/credential changes, EOF/Ctrl+C behavior, inherited ACL limits, safe context names, and the ARM64 runtime validation limit. Do not imply a Windows CLI needs a local Linux server or Make.
  - **Verify:** repeat documented commands on native Windows and record OS/shell/architecture; task 3 smoke script; `make test` on Linux before completion; scoped `golangci-lint run ./internal/testutil/... ./internal/config/... ./internal/idp/... ./internal/cli/... ./internal/climcp/... ./internal/web/... ./tools/bridgectl/...`; `npm --prefix ../erpbridge-docs run build`; `git diff --check` in both repositories. Run active LSP diagnostics on changed source paths during implementation. Do not commit generated schemas, binaries, or site build output.

## Verification and Acceptance

1. Windows x64 executes the focused suite and built-binary smoke checks, not only cross-compilation.
2. Both existing Windows architectures still compile and retain their ZIP names.
3. Test config, registry, and skill writes remain inside temporary directories. Runtime children cannot inherit real bridge credentials or a real user profile.
4. Unsafe Windows context identities fail before registry access or config replacement. Allowed contexts remain isolated. Linux context behavior stays unchanged.
5. Existing-file replacement, failure preservation, cleanup, Unicode/space paths, CLI exit codes, MCP framing/cancellation, and Console built assets have native evidence.
6. A non-admin user can install the ZIP and configure the CLI from PowerShell without Bash, Make, or a new installer.
7. Interactive browser/Ctrl+C evidence and ARM64 runtime limitations are reported separately from automated CI results.
8. Credentials, capability URLs, and raw sensitive outputs do not enter CI logs or recorded evidence. Windows ACL and rename limitations are stated accurately.
9. Linux behavior matches the recorded pre-change baseline. Add explicit Linux regression cases for existing valid context names including `CON`, `NUL.test`, and separate `prod`/`Prod` registries; unchanged config/registry paths and modes; environment precedence; CLI exit/output streams; and MCP startup, framing, pinning, cancellation, and SIGTERM shutdown. Run these with isolated fixtures and `go test -count=1` for the focused suite, followed by `make test`. Compare parsed protocol/structured results and filesystem effects, excluding only nondeterministic timestamps, temporary paths, and request IDs where appropriate. Do not weaken existing Linux tests or apply Windows skips to them.
10. Diff review confirms no changes to Linux-only runtime files, existing Linux workflows, Makefile, existing shell scripts, shared release configuration, deployment configuration, or dependencies. Shared source edits are limited to Windows-hook wiring with no-op non-Windows implementations; shared test edits retain Linux contracts.

## Checks Performed During Planning

- Read-only audits covered implementation, tests, release setup, workflows, documentation, and active/upcoming/stalled plan directories.
- `go test ./internal/config ./internal/bridgeclient ./internal/web ./internal/cli ./internal/climcp ./tools/bridgectl` passed on this Linux host; results were cached.
- The implementation audit cross-built Windows amd64 bridgectl and compiled Windows test binaries for `internal/cli` and `internal/climcp` successfully, with outputs in a temporary directory.
- Native Windows tests, ARM64 build/runtime checks, full `make test`, release publication checks, and public-site builds were not run during planning. No source code or deployment state was changed.

## Implementation Evidence

- Starting revision: `a622acd6ecb426edbd73d865a55dfd91c7f3dc78`. Work branch: `feat/bridgectl-windows-support`.
- User approved creating/pushing the dedicated branch and opening a draft PR on `nmdra/ERPBridge` to run native Windows checks in GitHub Actions. No merge or release is authorized.
- Before source edits, the uncached focused Linux suite and `make test` passed with an outer temporary home and preserved Go build caches. A pre-change CLI binary and parsed CLI/filesystem baseline were stored outside the repository under a temporary directory. Baseline covers context names `CON`, `NUL.test`, `prod`, `Prod`, config placeholder preservation and modes, independent case-sensitive registries, environment precedence, and error streams/exits. Existing MCP acceptance provided framing/pinning/cancellation/SIGTERM baseline coverage.
- Task 1 local red/green bootstrap, focused suite, and `make test` passed. All eight focused test packages cross-compiled for Windows amd64 and arm64. Native Windows execution is pending CI, so task 1 remains unchecked.
- The new config replacement-failure characterization test passes on Linux without production changes. It removes the staged file at the existing seam, then verifies original bytes and cleanup on the error path.

- Local Linux CLI/filesystem baseline comparison is byte-for-byte equal before and after the production hooks. Uncached focused tests, `make test`, and scoped golangci-lint passed. Existing Makefile, Linux workflows, shell scripts, shared release configuration, and dependencies are unchanged.
- Draft implementation PR: https://github.com/nmdra/ERPBridge/pull/21. Initial CI run https://github.com/nmdra/ERPBridge/actions/runs/39211547773 passed Linux compatibility, both Windows cross-builds, release-config validation, and the native home-isolation bootstrap. The existing Console workflow also passed. Native focused tests exposed a CRLF-only frontmatter fixture failure and an open invocation-server SQLite file at TempDir cleanup. The fixes normalize test input line endings and use an in-memory invocation-server fixture, retaining separate durable-store assertions. No production server changes or test skips are used.
- Source Windows guide and public-site mirror are implemented. `npm --prefix ../erpbridge-docs run build` passed; native documentation walkthrough and interactive browser/Ctrl+C gates remain pending. Task checkboxes remain open until all stated gates have evidence.

## Coordination and Approval

This plan does not supersede diagnose-and-recover, operational knowledge checkpoints, API-key propagation, live E2E fixes, or release plans. The diagnose-and-recover plan's copy snippets are explicitly POSIX-labelled; converting that planned UI to multi-shell output is not part of this work. Coordinate shared CLI tests and docs with one writer per file.

The working tree already contained unrelated modified/untracked plan and memory files. Preserve them. Plan directory contents are more complete than the indexes; refresh actual ownership and source references before execution. Verify public-docs instructions again before editing that repository.

Implementation and promotion to `active/` were approved. Native Windows verification gates may be deferred while local implementation proceeds, as explicitly approved by the user. Keep these gates visible and this plan active until they pass. Ask again before adding dependencies, custom ACL handling, storage migration, package managers, broader native server work, new public API contracts, paid runners, or release changes. If native tests expose a product defect outside the bounded contracts above, provide exact reproduction and request a plan amendment rather than silently expanding scope.

After all task gates pass, update the indexes, prefix the filename with `[COMPLETED]`, and archive it under `completed/`. Implementation is authorized by the user's approval. No release or live operation is authorized. Commits require the repository's commit safety checks, including explicit approval before committing on main.
